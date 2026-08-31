package com.kowerkoint.partnerwatch.connection

import android.content.Intent
import android.util.Log
import androidx.core.content.ContextCompat
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage

class PartnerFirebaseMessagingService : FirebaseMessagingService() {
    override fun onNewToken(token: String) {
        // Token registration with the Partner Watch API is added in the next step.
        Log.i(TAG, "FCM token refreshed")
    }

    override fun onMessageReceived(message: RemoteMessage) {
        if (message.data["type"] != "capture.wakeup") return
        Log.i(TAG, "FCM wakeup received")
        ContextCompat.startForegroundService(this, Intent(this, PartnerConnectionService::class.java))
    }

    private companion object { const val TAG = "PartnerWatchFCM" }
}
