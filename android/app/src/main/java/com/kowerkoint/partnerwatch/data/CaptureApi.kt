package com.kowerkoint.partnerwatch.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import java.io.IOException
import java.time.Instant

data class CreatedCaptureRequest(val requestId: String, val expiresAt: Instant)
data class CaptureRequestStatus(val requestId:String,val status:String,val imageId:String,val failure:String,val expiresAt:Instant)

sealed class CaptureRequestException(message: String) : IOException(message) {
    class RateLimited : CaptureRequestException("撮影要求の回数制限に達しました")
    class PartnerUnavailable : CaptureRequestException("相手の端末がまだ登録されていません")
    class Rejected : CaptureRequestException("撮影要求が拒否されました")
}

class CaptureApi(private val client: OkHttpClient = OkHttpClient()) {
    suspend fun create(session: DeviceSession): CreatedCaptureRequest = withContext(Dispatchers.IO) {
        val url = session.serverUrl.resolve("v1/capture-requests") ?: throw IOException("要求URLを作成できません")
        val request = Request.Builder().url(url).header("Authorization", "Bearer ${session.credential}")
            .post(ByteArray(0).toRequestBody(null)).build()
        client.newCall(request).execute().use { response ->
            when (response.code) {
                409 -> throw CaptureRequestException.PartnerUnavailable()
                429 -> throw CaptureRequestException.RateLimited()
            }
            if (response.code != 201) throw CaptureRequestException.Rejected()
            parseCreated(response.body.string())
        }
    }

    internal fun parseCreated(value: String): CreatedCaptureRequest = try {
        val json = JSONObject(value)
        CreatedCaptureRequest(json.getString("requestId"), Instant.parse(json.getString("expiresAt")))
            .also { require(it.requestId.isNotBlank()) }
    } catch (_: Exception) { throw CaptureRequestException.Rejected() }

    suspend fun status(session:DeviceSession,requestId:String):CaptureRequestStatus=withContext(Dispatchers.IO){
        val url=session.serverUrl.resolve("v1/capture-requests/$requestId")?:throw IOException("状態URLを作成できません")
        val request=Request.Builder().url(url).header("Authorization","Bearer ${session.credential}").get().build()
        client.newCall(request).execute().use{if(it.code!=200)throw IOException("撮影状態を取得できません");val j=JSONObject(it.body.string());CaptureRequestStatus(j.getString("requestId"),j.getString("status"),j.optString("imageId"),j.optString("failure"),Instant.parse(j.getString("expiresAt")))}
    }

    suspend fun reportReady(session: DeviceSession, requestId: String, imageId: String) =
        report(session, requestId, "READY", imageId, "")

    suspend fun reportFailure(session: DeviceSession, requestId: String, failure: String) =
        report(session, requestId, "FAILED", "", failure)

    private suspend fun report(
        session: DeviceSession,
        requestId: String,
        status: String,
        imageId: String,
        failure: String,
    ) = withContext(Dispatchers.IO) {
        val json = JSONObject().put("status", status).put("imageId", imageId).put("failure", failure)
        val url = session.serverUrl.resolve("v1/capture-requests/$requestId/result")
            ?: throw IOException("結果URLを作成できません")
        val request = Request.Builder().url(url)
            .header("Authorization", "Bearer ${session.credential}")
            .post(json.toString().toRequestBody(JSON)).build()
        client.newCall(request).execute().use {
            if (it.code != 204) throw IOException("結果報告が拒否されました: ${it.code}")
        }
    }

    private companion object { val JSON = "application/json; charset=utf-8".toMediaType() }
}
