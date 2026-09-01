package com.kowerkoint.partnerwatch.ui

import android.app.Application
import android.os.Build
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.kowerkoint.partnerwatch.data.EnrollmentApi
import com.kowerkoint.partnerwatch.data.EnrollmentException
import com.kowerkoint.partnerwatch.data.EnrollmentRepository
import com.kowerkoint.partnerwatch.data.EnrollmentStore
import com.kowerkoint.partnerwatch.data.SavedEnrollment
import com.kowerkoint.partnerwatch.capture.CapturePreferences
import com.kowerkoint.partnerwatch.capture.PartnerAccessibilityService
import com.kowerkoint.partnerwatch.connection.CaptureEventBus
import com.kowerkoint.partnerwatch.connection.CaptureCompletedEvent
import com.kowerkoint.partnerwatch.connection.ConnectionStatus
import com.kowerkoint.partnerwatch.connection.ConnectionStatusBus
import com.kowerkoint.partnerwatch.connection.FcmTokenRegistrar
import com.kowerkoint.partnerwatch.connection.ConnectionMode
import com.kowerkoint.partnerwatch.connection.ConnectionPreferences
import com.kowerkoint.partnerwatch.data.CaptureApi
import com.kowerkoint.partnerwatch.data.CaptureRequestException
import com.kowerkoint.partnerwatch.data.DeviceSessionRepository
import com.kowerkoint.partnerwatch.data.ImageApi
import com.kowerkoint.partnerwatch.data.ImageRepository
import com.kowerkoint.partnerwatch.data.PhotoCollection
import com.kowerkoint.partnerwatch.data.StatusApi
import com.kowerkoint.partnerwatch.data.PartnerBatteryStatus
import com.kowerkoint.partnerwatch.status.StatusPreferences
import com.kowerkoint.partnerwatch.connection.StatusEventBus
import com.kowerkoint.partnerwatch.security.DeviceSecurity
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.delay
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import java.io.IOException

data class EnrollmentForm(
    val serverUrl: String = "",
    val invitationCode: String = "",
    val deviceName: String = Build.MODEL,
    val isSubmitting: Boolean = false,
    val errorMessage: String? = null,
)

sealed interface EnrollmentUiState {
    data object Loading : EnrollmentUiState
    data class Form(val value: EnrollmentForm) : EnrollmentUiState
    data class Registered(
        val enrollment: SavedEnrollment,
        val acceptingCaptures: Boolean = false,
        val accessibilityConnected: Boolean = false,
        val connectionStatus: ConnectionStatus = ConnectionStatus.STARTING,
        val connectionMode: ConnectionMode = ConnectionMode.ALWAYS_CONNECTED,
        val capture: CaptureUiState = CaptureUiState.Idle,
        val sharingBattery: Boolean = false,
        val sharingLocation:Boolean=false,
        val preciseLocation:Boolean=false,
        val partnerBattery: BatteryUiState = BatteryUiState.Idle,
    ) : EnrollmentUiState
}

sealed interface BatteryUiState { data object Idle:BatteryUiState; data object Loading:BatteryUiState; data class Available(val value:PartnerBatteryStatus):BatteryUiState; data class Message(val text:String):BatteryUiState }

sealed interface CaptureUiState {
    data object Idle : CaptureUiState
    data class Waiting(val requestId: String) : CaptureUiState
    data class Received(val jpeg: ByteArray, val savedMessage: String? = null) : CaptureUiState
    data class Error(val message: String) : CaptureUiState
}

class EnrollmentViewModel(application: Application) : AndroidViewModel(application) {
    private val repository = EnrollmentRepository(
        api = EnrollmentApi(),
        store = EnrollmentStore(application.applicationContext),
        security = DeviceSecurity(),
    )
    private val capturePreferences = CapturePreferences(application.applicationContext)
    private val connectionPreferences = ConnectionPreferences(application.applicationContext)
    private val statusPreferences = StatusPreferences(application.applicationContext)
    private val statusApi = StatusApi()
    private val batteryState = MutableStateFlow<BatteryUiState>(BatteryUiState.Idle)
    private val security = DeviceSecurity()
    private val store = EnrollmentStore(application.applicationContext)
    private val sessions = DeviceSessionRepository(store, security)
    private val captureApi = CaptureApi()
    private val images = ImageRepository(ImageApi(), sessions)
    private val photos = PhotoCollection(application.applicationContext)
    private val captureState = MutableStateFlow<CaptureUiState>(CaptureUiState.Idle)
    private var timeoutJob: Job? = null
    private var registeredObservationJob: Job? = null
    private val earlyResults = mutableMapOf<String, CaptureCompletedEvent>()
    private val mutableState = MutableStateFlow<EnrollmentUiState>(EnrollmentUiState.Loading)
    val state: StateFlow<EnrollmentUiState> = mutableState.asStateFlow()

    init {
        viewModelScope.launch {
            CaptureEventBus.events.collect { event ->
                val waiting = captureState.value as? CaptureUiState.Waiting
                if (waiting?.requestId == event.requestId) applyCompletedEvent(event)
                else {
                    earlyResults[event.requestId] = event
                    while (earlyResults.size > 16) earlyResults.remove(earlyResults.keys.first())
                }
            }
        }
        viewModelScope.launch { StatusEventBus.events.collect { refreshPartnerBattery() } }
        viewModelScope.launch {
            val enrollment = repository.load()
            if (enrollment == null) {
                mutableState.value = EnrollmentUiState.Form(EnrollmentForm())
            } else {
                observeRegisteredState(enrollment)
            }
        }
    }

    fun updateServerUrl(value: String) = updateForm { copy(serverUrl = value, errorMessage = null) }

    fun updateInvitationCode(value: String) = updateForm { copy(invitationCode = value, errorMessage = null) }

    fun updateDeviceName(value: String) = updateForm { copy(deviceName = value, errorMessage = null) }

    fun enroll() {
        val form = (mutableState.value as? EnrollmentUiState.Form)?.value ?: return
        if (form.isSubmitting) return
        mutableState.value = EnrollmentUiState.Form(form.copy(isSubmitting = true, errorMessage = null))
        viewModelScope.launch {
            try {
                val enrollment = repository.enroll(form.serverUrl, form.invitationCode, form.deviceName)
                observeRegisteredState(enrollment)
            } catch (error: IllegalArgumentException) {
                showError(form, error.message ?: "入力内容を確認してください")
            } catch (error: EnrollmentException) {
                showError(form, error.message ?: "サーバーが登録要求を拒否しました")
            } catch (error: IOException) {
                showError(form, "サーバーへ接続できませんでした")
            } catch (_: Exception) {
                showError(form, "端末の登録に失敗しました")
            }
        }
    }

    fun setCaptureAccepting(value: Boolean) {
        if (mutableState.value !is EnrollmentUiState.Registered) return
        viewModelScope.launch { capturePreferences.setAccepting(value) }
    }

    fun setBatterySharing(value:Boolean) { viewModelScope.launch { statusPreferences.setSharingBattery(value); if(!value)runCatching{statusApi.clearOwnStatus(sessions.load(),"battery")} } }
    fun setLocationSharing(value:Boolean){viewModelScope.launch{statusPreferences.setSharingLocation(value);if(!value)runCatching{statusApi.clearOwnStatus(sessions.load(),"location")}}}
    fun setPreciseLocation(value:Boolean){viewModelScope.launch{statusPreferences.setPreciseLocation(value)}}
    fun requestPartnerStatus() { viewModelScope.launch {
        batteryState.value=BatteryUiState.Loading
        try {
            val session=sessions.load(); val started=java.time.Instant.now(); val created=statusApi.create(session)
            while (System.currentTimeMillis() < created.expiresAt.toEpochMilli()) {
                delay(1_000)
                statusApi.partner(session)?.takeIf { !it.reportedAt.isBefore(started) }?.let { batteryState.value=BatteryUiState.Available(it); return@launch }
            }
            batteryState.value=BatteryUiState.Message("状態更新がタイムアウトしました")
        } catch(e:Exception){ batteryState.value=BatteryUiState.Message(e.message?:"状態更新を要求できませんでした") }
    } }

    private suspend fun refreshPartnerBattery() {
        batteryState.value = try { statusApi.partner(sessions.load())?.let { BatteryUiState.Available(it) } ?: BatteryUiState.Message("相手の状態はまだ共有されていません") } catch (_:Exception) { BatteryUiState.Message("相手の状態を取得できませんでした") }
    }

    fun requestCapture() {
        if (captureState.value is CaptureUiState.Waiting) return
        viewModelScope.launch {
            try {
                val created = captureApi.create(sessions.load())
                captureState.value = CaptureUiState.Waiting(created.requestId)
                earlyResults.remove(created.requestId)?.let {
                    applyCompletedEvent(it)
                    return@launch
                }
                timeoutJob?.cancel()
                timeoutJob = viewModelScope.launch {
                    val session=sessions.load()
                    while (System.currentTimeMillis() <= created.expiresAt.toEpochMilli()+1_000 && (captureState.value as? CaptureUiState.Waiting)?.requestId==created.requestId) {
                        delay(1_000)
                        val result=runCatching{captureApi.status(session,created.requestId)}.getOrNull()?:continue
                        if(result.status!="PENDING"){
                            timeoutJob=null;applyCompletedEvent(CaptureCompletedEvent(result.requestId,result.status,result.imageId,result.failure));return@launch
                        }
                    }
                    if ((captureState.value as? CaptureUiState.Waiting)?.requestId == created.requestId) {
                        captureState.value = CaptureUiState.Error("撮影要求がタイムアウトしました")
                    }
                }
            } catch (error: CaptureRequestException) {
                captureState.value = CaptureUiState.Error(error.message ?: "撮影を要求できませんでした")
            } catch (_: Exception) {
                captureState.value = CaptureUiState.Error("サーバーへ接続できませんでした")
            }
        }
    }

    fun saveReceivedPhoto() {
        val received = captureState.value as? CaptureUiState.Received ?: return
        viewModelScope.launch {
            try {
                photos.save(received.jpeg)
                captureState.value = received.copy(savedMessage = "写真コレクションへ保存しました")
            } catch (_: Exception) {
                captureState.value = received.copy(savedMessage = "写真を保存できませんでした")
            }
        }
    }

    fun logout() {
        registeredObservationJob?.cancel()
        timeoutJob?.cancel()
        viewModelScope.launch {
            capturePreferences.setAccepting(false)
            statusPreferences.setSharingBattery(false)
            statusPreferences.setSharingLocation(false)
            store.clear()
            getApplication<Application>().stopService(android.content.Intent(getApplication(), com.kowerkoint.partnerwatch.connection.PartnerConnectionService::class.java))
            mutableState.value = EnrollmentUiState.Form(EnrollmentForm())
        }
    }

    fun disconnectForTest() {
        getApplication<Application>().stopService(
            android.content.Intent(getApplication(), com.kowerkoint.partnerwatch.connection.PartnerConnectionService::class.java),
        )
    }

    fun setConnectionMode(mode: ConnectionMode) {
        viewModelScope.launch { connectionPreferences.setMode(mode) }
    }

    private suspend fun observeRegisteredState(enrollment: SavedEnrollment) {
        runCatching { FcmTokenRegistrar.register(getApplication(), sessions) }
        runCatching { refreshPartnerBattery() }
        registeredObservationJob?.cancel()
        registeredObservationJob = viewModelScope.launch {
            val localSettings = combine(capturePreferences.accepting, statusPreferences.sharingBattery,statusPreferences.sharingLocation,statusPreferences.preciseLocation) { accepting, battery,location,precise -> listOf(accepting,battery,location,precise) }
            combine(localSettings, PartnerAccessibilityService.connected, ConnectionStatusBus.status, connectionPreferences.mode, combine(captureState,batteryState){capture,battery->capture to battery}) { settings, connected, connection, mode, remote ->
                EnrollmentUiState.Registered(enrollment, settings[0], connected, connection, mode, remote.first, settings[1],settings[2],settings[3], remote.second)
            }.collect { mutableState.value = it }
        }
        registeredObservationJob?.join()
    }

    private fun failureMessage(failure: String): String = when (failure) {
        "DISABLED" -> "相手が撮影受付を無効にしています"
        "SERVICE_UNAVAILABLE" -> "相手の撮影サービスが利用できません"
        "LOCKED" -> "相手の端末がロックされています"
        "CAPTURE_PROTECTED" -> "保護された画面のため撮影できません"
        "TIMEOUT" -> "撮影要求がタイムアウトしました"
        else -> "相手の端末で撮影に失敗しました"
    }

    private suspend fun applyCompletedEvent(event: CaptureCompletedEvent) {
        timeoutJob?.cancel()
        captureState.value = if (event.status == "READY" && event.imageId.isNotBlank()) {
            try { CaptureUiState.Received(images.download(event.imageId)) }
            catch (_: Exception) { CaptureUiState.Error("画像を取得できませんでした") }
        } else {
            CaptureUiState.Error(failureMessage(event.failure))
        }
    }

    private fun updateForm(transform: EnrollmentForm.() -> EnrollmentForm) {
        val current = (mutableState.value as? EnrollmentUiState.Form)?.value ?: return
        if (!current.isSubmitting) mutableState.value = EnrollmentUiState.Form(current.transform())
    }

    private fun showError(previous: EnrollmentForm, message: String) {
        mutableState.value = EnrollmentUiState.Form(previous.copy(isSubmitting = false, errorMessage = message))
    }
}
