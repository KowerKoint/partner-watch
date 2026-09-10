package com.kowerkoint.partnerwatch.notification

import android.app.Notification
import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import android.util.Log
import com.kowerkoint.partnerwatch.data.DeviceSessionRepository
import com.kowerkoint.partnerwatch.data.EnrollmentStore
import com.kowerkoint.partnerwatch.data.ForwardedNotificationApi
import com.kowerkoint.partnerwatch.security.DeviceSecurity
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import java.time.Instant
import java.util.concurrent.ConcurrentHashMap

class NotificationForwardingService : NotificationListenerService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val recent = ConcurrentHashMap<String, Long>()

    override fun onNotificationPosted(sbn: StatusBarNotification) {
        val notification = sbn.notification
        if (sbn.packageName == packageName || notification.flags and Notification.FLAG_ONGOING_EVENT != 0 || notification.flags and Notification.FLAG_GROUP_SUMMARY != 0) return
        val title = notification.extras.getCharSequence(Notification.EXTRA_TITLE)?.toString()?.trim().orEmpty().take(MAX_TITLE)
        val body = (notification.extras.getCharSequence(Notification.EXTRA_BIG_TEXT)
            ?: notification.extras.getCharSequence(Notification.EXTRA_TEXT))?.toString()?.trim().orEmpty().take(MAX_BODY)
        if (title.isBlank() && body.isBlank()) return
        val fingerprint = "${sbn.packageName}\u0000$title\u0000$body"
        val now = System.currentTimeMillis()
        recent.entries.removeIf { now - it.value > DEDUPE_WINDOW_MS }
        if (recent.put(fingerprint, now)?.let { now - it < DEDUPE_WINDOW_MS } == true) return

        scope.launch {
            if (!NotificationForwardingPreferences(applicationContext).enabled.first()) return@launch
            val appName = runCatching {
                val info = packageManager.getApplicationInfo(sbn.packageName, 0)
                packageManager.getApplicationLabel(info).toString()
            }.getOrDefault(sbn.packageName).take(MAX_APP_NAME)
            runCatching {
                val session = DeviceSessionRepository(EnrollmentStore(applicationContext), DeviceSecurity()).load()
                ForwardedNotificationApi().create(session, sbn.packageName.take(MAX_PACKAGE), appName, title, body, Instant.ofEpochMilli(sbn.postTime))
            }.onFailure { Log.w(TAG, "notification forwarding failed: ${it.javaClass.simpleName}") }
        }
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    private companion object {
        const val TAG = "PartnerWatchNotification"
        const val DEDUPE_WINDOW_MS = 5_000L
        const val MAX_APP_NAME = 120
        const val MAX_PACKAGE = 255
        const val MAX_TITLE = 500
        const val MAX_BODY = 4_000
    }
}
