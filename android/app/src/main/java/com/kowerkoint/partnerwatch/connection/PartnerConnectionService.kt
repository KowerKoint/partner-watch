package com.kowerkoint.partnerwatch.connection

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.graphics.BitmapFactory
import android.os.IBinder
import android.os.BatteryManager
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import com.kowerkoint.partnerwatch.MainActivity
import com.kowerkoint.partnerwatch.R
import com.kowerkoint.partnerwatch.capture.CaptureFailure
import com.kowerkoint.partnerwatch.capture.CapturePreferences
import com.kowerkoint.partnerwatch.capture.CaptureRejectedException
import com.kowerkoint.partnerwatch.capture.CaptureUploader
import com.kowerkoint.partnerwatch.capture.CapturedUpload
import com.kowerkoint.partnerwatch.capture.JpegEncoder
import com.kowerkoint.partnerwatch.capture.ScreenshotCaptureException
import com.kowerkoint.partnerwatch.data.CaptureApi
import com.kowerkoint.partnerwatch.data.DeviceSession
import com.kowerkoint.partnerwatch.data.DeviceSessionRepository
import com.kowerkoint.partnerwatch.data.EnrollmentStore
import com.kowerkoint.partnerwatch.data.ImageApi
import com.kowerkoint.partnerwatch.data.ImageRepository
import com.kowerkoint.partnerwatch.data.PendingCaptureApi
import com.kowerkoint.partnerwatch.data.StatusApi
import com.kowerkoint.partnerwatch.status.StatusPreferences
import com.kowerkoint.partnerwatch.security.DeviceSecurity
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import java.time.Instant
import kotlin.math.min

internal data class CaptureRequestedEvent(val requestId: String, val expiresAt: Instant)
internal data class StatusRequestedEvent(val requestId: String, val expiresAt: Instant)

internal fun parseCaptureRequestedEvent(text: String): CaptureRequestedEvent? {
    val json = runCatching { JSONObject(text) }.getOrNull() ?: return null
    if (json.optString("type") != "capture.requested") return null
    val requestId = json.optString("requestId")
    val expiresAt = runCatching { Instant.parse(json.optString("expiresAt")) }.getOrNull() ?: return null
    return CaptureRequestedEvent(requestId, expiresAt).takeIf { requestId.isNotBlank() }
}

internal fun parseStatusRequestedEvent(text: String): StatusRequestedEvent? {
    val json = runCatching { JSONObject(text) }.getOrNull() ?: return null
    if (json.optString("type") != "status.requested") return null
    val requestId = json.optString("requestId")
    val expiresAt = runCatching { Instant.parse(json.optString("expiresAt")) }.getOrNull() ?: return null
    return StatusRequestedEvent(requestId, expiresAt).takeIf { requestId.isNotBlank() }
}

internal fun parseCaptureCompletedEvent(text: String): CaptureCompletedEvent? {
    val json = runCatching { JSONObject(text) }.getOrNull() ?: return null
    if (json.optString("type") != "capture.completed") return null
    return CaptureCompletedEvent(
        json.optString("requestId"), json.optString("status"),
        json.optString("imageId"), json.optString("failure"),
    ).takeIf { it.requestId.isNotBlank() && it.status in setOf("READY", "FAILED", "TIMEOUT") }
}

class PartnerConnectionService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val client = OkHttpClient()
    private val captureMutex = Mutex()
    private lateinit var sessions: DeviceSessionRepository
    private lateinit var uploader: CaptureUploader
    private lateinit var notificationManager: NotificationManager
    private lateinit var pendingIntent: PendingIntent
    private val captureApi = CaptureApi(client)
    private val pendingCaptureApi = PendingCaptureApi(client)
    private val statusApi = StatusApi(client)
    private lateinit var statusPreferences: StatusPreferences
    private var connectionJob: Job? = null
    private var oneShotWakeup = false

    override fun onCreate() {
        super.onCreate()
        val security = DeviceSecurity()
        val store = EnrollmentStore(applicationContext)
        sessions = DeviceSessionRepository(store, security)
        statusPreferences = StatusPreferences(applicationContext)
        uploader = CaptureUploader(
            applicationContext, CapturePreferences(applicationContext), JpegEncoder(),
            ImageRepository(ImageApi(client), sessions),
        )
        notificationManager = getSystemService(NotificationManager::class.java)
        createNotificationChannel()
        createCaptureNotificationChannel()
        createStatusNotificationChannel()
        pendingIntent = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE,
        )
        ConnectionStatusBus.set(ConnectionStatus.STARTING)
        val notification = buildNotification(ConnectionStatus.STARTING)
        ServiceCompat.startForeground(
            this, NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,
        )
        connectionJob = scope.launch { connectionLoop() }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        oneShotWakeup = intent?.action == ACTION_FCM_WAKEUP
        return START_NOT_STICKY
    }
    override fun onBind(intent: Intent?): IBinder? = null
    override fun onDestroy() { scope.cancel(); super.onDestroy() }

    private suspend fun connectionLoop() {
        var backoff = 1_000L
        while (scope.isActive) {
            try {
                connectOnce(sessions.load())
                delay(1_000L)
                backoff = 1_000L
            } catch (error: Exception) {
                updateConnectionStatus(ConnectionStatus.RECONNECTING)
                Log.w(TAG, "event connection failed: ${error.javaClass.simpleName}: ${error.message}")
                delay(backoff)
                backoff = min(backoff * 2, 60_000L)
            }
        }
    }

    private suspend fun connectOnce(session: DeviceSession) {
        val httpUrl = session.serverUrl.resolve("v1/events") ?: error("イベントURLが不正です")
        // OkHttp's WebSocket client accepts an http/https URL and performs the upgrade itself.
        val wsUrl = httpUrl.newBuilder().scheme("https").build()
        val closed = CompletableDeferred<Unit>()
        val request = Request.Builder().url(wsUrl)
            .header("Authorization", "Bearer ${session.credential}").build()
        val socket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                updateConnectionStatus(ConnectionStatus.CONNECTED)
                scope.launch { processPendingRequests(session) }
            }
            override fun onMessage(webSocket: WebSocket, text: String) { handleEvent(session, text) }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                updateConnectionStatus(ConnectionStatus.RECONNECTING)
                closed.complete(Unit)
            }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                updateConnectionStatus(ConnectionStatus.RECONNECTING)
                Log.w(TAG, "event connection failed: ${t.javaClass.simpleName}")
                closed.completeExceptionally(t)
            }
        })
        try { closed.await() } finally { socket.cancel() }
    }

    private fun handleEvent(session: DeviceSession, text: String) {
        parseCaptureCompletedEvent(text)?.let { CaptureEventBus.publish(it); return }
        val completedJson = runCatching { JSONObject(text) }.getOrNull()
        if (completedJson?.optString("type") == "status.completed") {
            StatusEventBus.publish(StatusCompletedEvent(completedJson.optString("requestId")))
            return
        }
        parseStatusRequestedEvent(text)?.let { event ->
            if (event.expiresAt.isAfter(Instant.now())) scope.launch { processStatus(session, event.requestId) }
            return
        }
        val event = parseCaptureRequestedEvent(text) ?: return
        if (!event.expiresAt.isAfter(Instant.now())) return
        scope.launch {
            captureMutex.withLock {
                if (event.expiresAt.isAfter(Instant.now())) processCapture(session, event.requestId)
            }
            if (oneShotWakeup) stopSelf()
        }
    }

    private suspend fun processPendingRequests(session: DeviceSession) {
        runCatching { pendingCaptureApi.list(session) }.getOrDefault(emptyList()).forEach { event ->
            if (event.expiresAt.isAfter(Instant.now())) {
                captureMutex.withLock { if (event.expiresAt.isAfter(Instant.now())) processCapture(session, event.requestId) }
            }
        }
        runCatching { statusApi.pending(session) }.getOrDefault(emptyList()).forEach { event ->
            if (event.expiresAt.isAfter(Instant.now())) processStatus(session, event.requestId)
        }
        if (oneShotWakeup) stopSelf()
    }

    private suspend fun processStatus(session: DeviceSession, requestId: String) {
        val enabled = statusPreferences.isSharingBattery()
        val manager = getSystemService(BatteryManager::class.java)
        val percent = manager.getIntProperty(BatteryManager.BATTERY_PROPERTY_CAPACITY).coerceIn(0, 100)
        val charging = when (manager.getIntProperty(BatteryManager.BATTERY_PROPERTY_STATUS)) {
            BatteryManager.BATTERY_STATUS_CHARGING -> "CHARGING"
            BatteryManager.BATTERY_STATUS_DISCHARGING -> "DISCHARGING"
            BatteryManager.BATTERY_STATUS_FULL -> "FULL"
            BatteryManager.BATTERY_STATUS_NOT_CHARGING -> "NOT_CHARGING"
            else -> "UNKNOWN"
        }
        runCatching { statusApi.reportBattery(session, requestId, enabled, percent, charging) }
            .onSuccess { showStatusSharedNotification(enabled, percent) }
    }

    private suspend fun processCapture(session: DeviceSession, requestId: String) {
        try {
            val uploaded = uploader.captureAndUpload()
            captureApi.reportReady(session, requestId, uploaded.uploaded.imageId)
            showCaptureNotification(uploaded)
        } catch (_: CaptureRejectedException) {
            captureApi.reportFailure(session, requestId, "DISABLED")
        } catch (error: ScreenshotCaptureException) {
            val failure = when (error.failure) {
                CaptureFailure.SERVICE_UNAVAILABLE -> "SERVICE_UNAVAILABLE"
                CaptureFailure.CAPTURE_PROTECTED -> "CAPTURE_PROTECTED"
                CaptureFailure.LOCKED -> "LOCKED"
                else -> "INTERNAL_ERROR"
            }
            captureApi.reportFailure(session, requestId, failure)
        } catch (_: Exception) {
            runCatching { captureApi.reportFailure(session, requestId, "INTERNAL_ERROR") }
        }
    }

    private fun createNotificationChannel() {
        getSystemService(NotificationManager::class.java).createNotificationChannel(
            NotificationChannel(CHANNEL_ID, "Partner Watch接続", NotificationManager.IMPORTANCE_LOW),
        )
    }

    private fun createCaptureNotificationChannel() {
        notificationManager.createNotificationChannel(
            NotificationChannel(CAPTURE_CHANNEL_ID, "撮影結果", NotificationManager.IMPORTANCE_DEFAULT),
        )
    }

    private fun createStatusNotificationChannel() {
        notificationManager.createNotificationChannel(NotificationChannel(STATUS_CHANNEL_ID, "状態共有", NotificationManager.IMPORTANCE_DEFAULT))
    }

    private fun showStatusSharedNotification(enabled:Boolean,percent:Int) {
        val text=if(enabled)"バッテリー残量（$percent%）を共有しました" else "バッテリーは共有しませんでした"
        notificationManager.notify(STATUS_NOTIFICATION_ID,NotificationCompat.Builder(this,STATUS_CHANNEL_ID).setSmallIcon(R.drawable.ic_launcher_foreground).setContentTitle("相手から状態の更新要求がありました").setContentText(text).setAutoCancel(true).setContentIntent(pendingIntent).build())
    }

    private fun showCaptureNotification(capture: CapturedUpload) {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(capture.jpeg, 0, capture.jpeg.size, bounds)
        val sample = generateSequence(1) { it * 2 }
            .takeWhile { it <= 16 }
            .lastOrNull { bounds.outWidth / it >= 512 || bounds.outHeight / it >= 512 } ?: 1
        val options = BitmapFactory.Options().apply { inSampleSize = sample }
        val bitmap = BitmapFactory.decodeByteArray(capture.jpeg, 0, capture.jpeg.size, options) ?: return
        val notification = NotificationCompat.Builder(this, CAPTURE_CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_launcher_foreground)
            .setContentTitle("画面を撮影しました")
            .setContentText("撮影側の画面を確認してください")
            .setStyle(NotificationCompat.BigPictureStyle().bigPicture(bitmap))
            .setAutoCancel(true)
            .setContentIntent(pendingIntent)
            .build()
        notificationManager.notify(CAPTURE_NOTIFICATION_ID, notification)
    }

    private fun updateConnectionStatus(status: ConnectionStatus) {
        ConnectionStatusBus.set(status)
        if (::notificationManager.isInitialized && ::pendingIntent.isInitialized) {
            notificationManager.notify(NOTIFICATION_ID, buildNotification(status))
        }
    }

    private fun buildNotification(status: ConnectionStatus) = NotificationCompat.Builder(this, CHANNEL_ID)
        .setSmallIcon(R.drawable.ic_launcher_foreground)
        .setContentTitle("Partner Watch")
        .setContentText(when (status) {
            ConnectionStatus.STARTING -> "サーバーへ接続しています"
            ConnectionStatus.CONNECTED -> "サーバー接続済み・撮影要求を待機しています"
            ConnectionStatus.RECONNECTING -> "サーバー再接続中"
        })
        .setOngoing(true).setContentIntent(pendingIntent).build()

    companion object {
        const val ACTION_FCM_WAKEUP = "com.kowerkoint.partnerwatch.action.FCM_WAKEUP"
        const val TAG = "PartnerWatchConnection"
        const val CHANNEL_ID = "partner_connection"
        const val NOTIFICATION_ID = 1001
        const val CAPTURE_CHANNEL_ID = "capture_result"
        const val CAPTURE_NOTIFICATION_ID = 1002
        const val STATUS_CHANNEL_ID = "status_shared"
        const val STATUS_NOTIFICATION_ID = 1003
    }
}
