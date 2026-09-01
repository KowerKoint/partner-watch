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

data class CreatedStatusRequest(val requestId: String, val expiresAt: Instant)
data class PartnerBatteryStatus(val status: String, val percent: Int?, val chargingState: String?, val reportedAt: Instant)

class StatusApi(private val client: OkHttpClient = OkHttpClient()) {
    suspend fun create(session: DeviceSession): CreatedStatusRequest = withContext(Dispatchers.IO) {
        val response = execute(session, "v1/status-requests", Request.Builder().post(ByteArray(0).toRequestBody(null)))
        response.use { if (it.code == 429) throw IOException("状態更新は1分後に再試行してください"); if (it.code != 201) throw IOException("状態更新を要求できませんでした"); val json=JSONObject(it.body.string()); CreatedStatusRequest(json.getString("requestId"),Instant.parse(json.getString("expiresAt"))) }
    }
    suspend fun pending(session: DeviceSession): List<CreatedStatusRequest> = withContext(Dispatchers.IO) {
        execute(session,"v1/status-requests/pending",Request.Builder().get()).use { r->if(r.code!=200)throw IOException("保留要求を取得できません");val a=JSONObject(r.body.string()).getJSONArray("requests");(0 until a.length()).map{val j=a.getJSONObject(it);CreatedStatusRequest(j.getString("requestId"),Instant.parse(j.getString("expiresAt")))}}
    }
    suspend fun reportBattery(session: DeviceSession, requestId: String, enabled: Boolean, percent: Int, charging: String) = withContext(Dispatchers.IO) {
        val battery=JSONObject().put("status",if(enabled)"AVAILABLE" else "DISABLED").put("percent",if(enabled)percent else 0).put("chargingState",if(enabled)charging else "")
        execute(session,"v1/status-requests/$requestId/result",Request.Builder().post(JSONObject().put("battery",battery).toString().toRequestBody(JSON))).use{if(it.code!=204)throw IOException("状態を報告できませんでした")}
    }
    suspend fun partner(session: DeviceSession): PartnerBatteryStatus? = withContext(Dispatchers.IO) {
        execute(session,"v1/partner-status",Request.Builder().get()).use{if(it.code==404)return@withContext null;if(it.code!=200)throw IOException("相手の状態を取得できません");val j=JSONObject(it.body.string());val b=j.getJSONObject("battery");PartnerBatteryStatus(b.getString("status"),if(b.getString("status")=="AVAILABLE")b.getInt("percent")else null,b.optString("chargingState").ifBlank{null},Instant.parse(j.getString("reportedAt")))}
    }
    suspend fun clearOwnStatus(session: DeviceSession) = withContext(Dispatchers.IO) { execute(session,"v1/device-status",Request.Builder().delete()).use { if(it.code!=204)throw IOException("共有済み状態を削除できませんでした") } }
    private fun execute(session:DeviceSession,path:String,builder:Request.Builder)=client.newCall(builder.url(session.serverUrl.resolve(path)?:throw IOException("URLが不正です")).header("Authorization","Bearer ${session.credential}").build()).execute()
    private companion object { val JSON="application/json; charset=utf-8".toMediaType() }
}
