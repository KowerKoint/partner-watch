package com.kowerkoint.partnerwatch.connection

import android.content.Intent
import android.util.Log
import androidx.core.content.ContextCompat
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import com.kowerkoint.partnerwatch.data.DeviceSessionRepository
import com.kowerkoint.partnerwatch.data.EnrollmentStore
import com.kowerkoint.partnerwatch.data.FcmTokenApi
import com.kowerkoint.partnerwatch.security.DeviceSecurity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

class PartnerFirebaseMessagingService : FirebaseMessagingService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onNewToken(token: String) {
        Log.i(TAG, "FCM token refreshed")
        scope.launch {
            runCatching {
                FcmTokenApi().register(
                    DeviceSessionRepository(EnrollmentStore(applicationContext), DeviceSecurity()).load(), token,
                )
            }.onFailure { Log.w(TAG, "FCM token registration failed") }
        }
    }

    override fun onMessageReceived(message: RemoteMessage) {
        if (message.data["type"] != "capture.wakeup") return
        Log.i(TAG, "FCM wakeup received")
        ContextCompat.startForegroundService(this, Intent(this, PartnerConnectionService::class.java).setAction(PartnerConnectionService.ACTION_FCM_WAKEUP))
    }

    override fun onDestroy() { scope.cancel(); super.onDestroy() }

    private companion object { const val TAG = "PartnerWatchFCM" }
}
