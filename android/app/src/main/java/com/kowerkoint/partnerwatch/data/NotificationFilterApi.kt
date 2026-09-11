package com.kowerkoint.partnerwatch.data

import android.content.Context
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.text.Normalizer
import java.util.Locale

enum class TextMatch { CONTAINS, EXACT }

data class NotificationFilter(
    val id: String = "",
    val sourceDeviceId: String = "",
    val sourceDeviceName: String = "",
    val sourcePackage: String = "",
    val sourceAppName: String = "",
    val titlePattern: String = "",
    val titleMatch: TextMatch = TextMatch.CONTAINS,
    val messagePattern: String = "",
    val messageMatch: TextMatch = TextMatch.CONTAINS,
    val enabled: Boolean = true,
) {
    fun matches(item: ForwardedNotification): Boolean {
        if (!enabled || sourceDeviceId.isNotEmpty() && sourceDeviceId != item.sourceDeviceId || sourcePackage.isNotEmpty() && sourcePackage != item.sourcePackage) return false
        if (titlePattern.isNotEmpty() && !textMatches(item.title, titlePattern, titleMatch)) return false
        return messagePattern.isEmpty() || textMatches(item.title + "\n" + item.body, messagePattern, messageMatch)
    }
}

private fun normalized(value: String) = Normalizer.normalize(value, Normalizer.Form.NFKC).lowercase(Locale.ROOT)
private fun textMatches(value: String, pattern: String, mode: TextMatch): Boolean = when (mode) {
    TextMatch.EXACT -> normalized(value) == normalized(pattern)
    TextMatch.CONTAINS -> normalized(value).contains(normalized(pattern))
}

class NotificationFilterApi(private val client: OkHttpClient = OkHttpClient()) {
    suspend fun list(session: DeviceSession): List<NotificationFilter> = withContext(Dispatchers.IO) {
        execute(session, "v1/notification-filters", Request.Builder().get()).use { response ->
            if (response.code != 200) throw IOException("通知フィルターを取得できませんでした (${response.code})")
            val values = JSONObject(response.body.string()).getJSONArray("filters")
            (0 until values.length()).map { decode(values.getJSONObject(it)) }
        }
    }

    suspend fun save(session: DeviceSession, value: NotificationFilter): NotificationFilter = withContext(Dispatchers.IO) {
        val body = encode(value).toString().toRequestBody(JSON)
        val path = if (value.id.isEmpty()) "v1/notification-filters" else "v1/notification-filters/${value.id}"
        val builder = if (value.id.isEmpty()) Request.Builder().post(body) else Request.Builder().put(body)
        execute(session, path, builder).use { response ->
            if (response.code !in setOf(200, 201)) throw IOException("通知フィルターを保存できませんでした (${response.code})")
            decode(JSONObject(response.body.string()))
        }
    }

    suspend fun delete(session: DeviceSession, id: String) = withContext(Dispatchers.IO) {
        execute(session, "v1/notification-filters/$id", Request.Builder().delete()).use {
            if (it.code != 204 && it.code != 404) throw IOException("通知フィルターを削除できませんでした (${it.code})")
        }
    }

    private fun execute(session: DeviceSession, path: String, builder: Request.Builder) = client.newCall(builder
        .url(session.serverUrl.resolve(path) ?: throw IOException("URLが不正です"))
        .header("Authorization", "Bearer ${session.credential}").build()).execute()

    private fun encode(v: NotificationFilter) = JSONObject().put("sourceDeviceId", v.sourceDeviceId).put("sourceDeviceName", v.sourceDeviceName)
        .put("sourcePackage", v.sourcePackage).put("sourceAppName", v.sourceAppName).put("titlePattern", v.titlePattern)
        .put("titleMatch", v.titleMatch.name).put("messagePattern", v.messagePattern).put("messageMatch", v.messageMatch.name).put("enabled", v.enabled)
    private fun decode(v: JSONObject) = NotificationFilter(v.optString("id"), v.optString("sourceDeviceId"), v.optString("sourceDeviceName"),
        v.optString("sourcePackage"), v.optString("sourceAppName"), v.optString("titlePattern"), enumValue(v.optString("titleMatch")),
        v.optString("messagePattern"), enumValue(v.optString("messageMatch")), v.optBoolean("enabled", true))
    private fun enumValue(value: String) = runCatching { TextMatch.valueOf(value) }.getOrDefault(TextMatch.CONTAINS)
    private companion object { val JSON = "application/json; charset=utf-8".toMediaType() }
}

class NotificationFilterCache(context: Context) {
    private val preferences = context.getSharedPreferences("notification_filter_cache", Context.MODE_PRIVATE)
    fun load(): List<NotificationFilter> = runCatching {
        val values = JSONArray(preferences.getString("filters", "[]"))
        (0 until values.length()).map { value ->
            val v=values.getJSONObject(value); NotificationFilter(v.optString("id"),v.optString("sourceDeviceId"),v.optString("sourceDeviceName"),v.optString("sourcePackage"),v.optString("sourceAppName"),v.optString("titlePattern"),runCatching{TextMatch.valueOf(v.optString("titleMatch"))}.getOrDefault(TextMatch.CONTAINS),v.optString("messagePattern"),runCatching{TextMatch.valueOf(v.optString("messageMatch"))}.getOrDefault(TextMatch.CONTAINS),v.optBoolean("enabled",true))
        }
    }.getOrDefault(emptyList())
    fun save(values: List<NotificationFilter>) { val array=JSONArray();values.forEach{v->array.put(JSONObject().put("id",v.id).put("sourceDeviceId",v.sourceDeviceId).put("sourceDeviceName",v.sourceDeviceName).put("sourcePackage",v.sourcePackage).put("sourceAppName",v.sourceAppName).put("titlePattern",v.titlePattern).put("titleMatch",v.titleMatch.name).put("messagePattern",v.messagePattern).put("messageMatch",v.messageMatch.name).put("enabled",v.enabled))};preferences.edit().putString("filters",array.toString()).apply() }
    fun clear() { preferences.edit().clear().apply() }
}
