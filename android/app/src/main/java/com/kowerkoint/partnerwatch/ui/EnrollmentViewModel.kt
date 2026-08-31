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
import com.kowerkoint.partnerwatch.security.DeviceSecurity
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
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
    data class Registered(val enrollment: SavedEnrollment) : EnrollmentUiState
}

class EnrollmentViewModel(application: Application) : AndroidViewModel(application) {
    private val repository = EnrollmentRepository(
        api = EnrollmentApi(),
        store = EnrollmentStore(application.applicationContext),
        security = DeviceSecurity(),
    )
    private val mutableState = MutableStateFlow<EnrollmentUiState>(EnrollmentUiState.Loading)
    val state: StateFlow<EnrollmentUiState> = mutableState.asStateFlow()

    init {
        viewModelScope.launch {
            mutableState.value = repository.load()?.let(EnrollmentUiState::Registered)
                ?: EnrollmentUiState.Form(EnrollmentForm())
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
                mutableState.value = EnrollmentUiState.Registered(enrollment)
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

    private fun updateForm(transform: EnrollmentForm.() -> EnrollmentForm) {
        val current = (mutableState.value as? EnrollmentUiState.Form)?.value ?: return
        if (!current.isSubmitting) mutableState.value = EnrollmentUiState.Form(current.transform())
    }

    private fun showError(previous: EnrollmentForm, message: String) {
        mutableState.value = EnrollmentUiState.Form(previous.copy(isSubmitting = false, errorMessage = message))
    }
}
