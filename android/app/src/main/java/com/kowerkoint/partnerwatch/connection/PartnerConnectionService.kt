package com.kowerkoint.partnerwatch.connection

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.IBinder
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import com.kowerkoint.partnerwatch.MainActivity
import com.kowerkoint.partnerwatch.R
import com.kowerkoint.partnerwatch.capture.CaptureFailure
import com.kowerkoint.partnerwatch.capture.CapturePreferences
import com.kowerkoint.partnerwatch.capture.CaptureRejectedException
import com.kowerkoint.partnerwatch.capture.CaptureUploader
import com.kowerkoint.partnerwatch.capture.JpegEncoder
import com.kowerkoint.partnerwatch.capture.ScreenshotCaptureException
import com.kowerkoint.partnerwatch.data.CaptureApi
import com.kowerkoint.partnerwatch.data.DeviceSession
import com.kowerkoint.partnerwatch.data.DeviceSessionRepository
import com.kowerkoint.partnerwatch.data.EnrollmentStore
import com.kowerkoint.partnerwatch.data.ImageApi
import com.kowerkoint.partnerwatch.data.ImageRepository
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

internal fun parseCaptureRequestedEvent(text: String): CaptureRequestedEvent? {
    val json = runCatching { JSONObject(text) }.getOrNull() ?: return null
    if (json.optString("type") != "capture.requested") return null
    val requestId = json.optString("requestId")
    val expiresAt = runCatching { Instant.parse(json.optString("expiresAt")) }.getOrNull() ?: return null
    return CaptureRequestedEvent(requestId, expiresAt).takeIf { requestId.isNotBlank() }
}

class PartnerConnectionService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val client = OkHttpClient()
    private val captureMutex = Mutex()
    private lateinit var sessions: DeviceSessionRepository
    private lateinit var uploader: CaptureUploader
    private val captureApi = CaptureApi(client)
    private var connectionJob: Job? = null

    override fun onCreate() {
        super.onCreate()
        val security = DeviceSecurity()
        val store = EnrollmentStore(applicationContext)
        sessions = DeviceSessionRepository(store, security)
        uploader = CaptureUploader(
            applicationContext, CapturePreferences(applicationContext), JpegEncoder(),
            ImageRepository(ImageApi(client), sessions),
        )
        createNotificationChannel()
        val pendingIntent = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_launcher_foreground)
            .setContentTitle("Partner Watch")
            .setContentText("撮影要求を待機しています")
            .setOngoing(true).setContentIntent(pendingIntent).build()
        ServiceCompat.startForeground(
            this, NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE,
        )
        connectionJob = scope.launch { connectionLoop() }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int = START_STICKY
    override fun onBind(intent: Intent?): IBinder? = null
    override fun onDestroy() { scope.cancel(); super.onDestroy() }

    private suspend fun connectionLoop() {
        var backoff = 1_000L
        while (scope.isActive) {
            try {
                connectOnce(sessions.load())
                delay(1_000L)
                backoff = 1_000L
            } catch (_: Exception) {
                delay(backoff)
                backoff = min(backoff * 2, 60_000L)
            }
        }
    }

    private suspend fun connectOnce(session: DeviceSession) {
        val httpUrl = session.serverUrl.resolve("v1/events") ?: error("イベントURLが不正です")
        val wsUrl = httpUrl.newBuilder().scheme("wss").build()
        val closed = CompletableDeferred<Unit>()
        val request = Request.Builder().url(wsUrl)
            .header("Authorization", "Bearer ${session.credential}").build()
        val socket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onMessage(webSocket: WebSocket, text: String) { handleEvent(session, text) }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) { closed.complete(Unit) }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                closed.completeExceptionally(t)
            }
        })
        try { closed.await() } finally { socket.cancel() }
    }

    private fun handleEvent(session: DeviceSession, text: String) {
        val event = parseCaptureRequestedEvent(text) ?: return
        if (!event.expiresAt.isAfter(Instant.now())) return
        scope.launch {
            captureMutex.withLock {
                if (event.expiresAt.isAfter(Instant.now())) processCapture(session, event.requestId)
            }
        }
    }

    private suspend fun processCapture(session: DeviceSession, requestId: String) {
        try {
            val uploaded = uploader.captureAndUpload()
            captureApi.reportReady(session, requestId, uploaded.imageId)
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

    private companion object { const val CHANNEL_ID = "partner_connection"; const val NOTIFICATION_ID = 1001 }
}
