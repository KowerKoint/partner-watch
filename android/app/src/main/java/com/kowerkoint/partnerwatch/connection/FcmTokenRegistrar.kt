package com.kowerkoint.partnerwatch.connection

import android.content.Context
import com.google.firebase.messaging.FirebaseMessaging
import com.kowerkoint.partnerwatch.data.DeviceSessionRepository
import com.kowerkoint.partnerwatch.data.FcmTokenApi
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

object FcmTokenRegistrar {
    suspend fun register(context: Context, sessions: DeviceSessionRepository) {
        val token = suspendCancellableCoroutine<String> { continuation ->
            FirebaseMessaging.getInstance().token.addOnCompleteListener { task ->
                if (task.isSuccessful) continuation.resume(task.result)
                else continuation.resumeWithException(task.exception ?: IllegalStateException("FCM token unavailable"))
            }
        }
        FcmTokenApi().register(sessions.load(), token)
    }
}
