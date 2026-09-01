package com.kowerkoint.partnerwatch.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import java.time.Instant

data class PendingCapture(val requestId: String, val expiresAt: Instant)

class PendingCaptureApi(private val client: OkHttpClient = OkHttpClient()) {
    suspend fun list(session: DeviceSession): List<PendingCapture> = withContext(Dispatchers.IO) {
        val url = session.serverUrl.resolve("v1/capture-requests/pending") ?: error("保留要求URLを作成できません")
        val request = Request.Builder().url(url).header("Authorization", "Bearer ${session.credential}").get().build()
        client.newCall(request).execute().use { response ->
            if (!response.isSuccessful) error("保留要求を取得できませんでした")
            val json = JSONObject(response.body.string())
            val items = json.optJSONArray("requests") ?: return@withContext emptyList()
            (0 until items.length()).mapNotNull { index ->
                val item = items.optJSONObject(index) ?: return@mapNotNull null
                val id = item.optString("requestId")
                val expires = runCatching { Instant.parse(item.optString("expiresAt")) }.getOrNull()
                if (id.isBlank() || expires == null) null else PendingCapture(id, expires)
            }
        }
    }
}
