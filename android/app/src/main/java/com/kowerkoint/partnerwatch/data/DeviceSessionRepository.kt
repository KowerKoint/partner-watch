package com.kowerkoint.partnerwatch.data

import com.kowerkoint.partnerwatch.security.DeviceSecurity
import okhttp3.HttpUrl

data class DeviceSession(val serverUrl: HttpUrl, val credential: String)

class DeviceSessionRepository(
    private val store: EnrollmentStore,
    private val security: DeviceSecurity,
) {
    suspend fun load(): DeviceSession {
        val enrollment = store.load() ?: error("端末が登録されていません")
        val encrypted = store.loadEncryptedCredential() ?: error("認証情報がありません")
        val endpoint = ServerEndpoint.parse(enrollment.serverUrl) ?: error("保存済みサーバーURLが不正です")
        return DeviceSession(endpoint, security.decryptCredential(encrypted))
    }
}
