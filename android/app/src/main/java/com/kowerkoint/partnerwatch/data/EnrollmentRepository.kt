package com.kowerkoint.partnerwatch.data

import com.kowerkoint.partnerwatch.security.DeviceSecurity
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

class EnrollmentRepository(
    private val api: EnrollmentApi,
    private val store: EnrollmentStore,
    private val security: DeviceSecurity,
) {
    suspend fun load(): SavedEnrollment? = store.load()

    suspend fun enroll(serverUrl: String, invitationToken: String, deviceName: String): SavedEnrollment {
        val endpoint = ServerEndpoint.parse(serverUrl) ?: throw IllegalArgumentException("HTTPSのサーバーURLを入力してください")
        val normalizedName = deviceName.trim()
        require(normalizedName.isNotEmpty() && normalizedName.length <= 80) {
            "端末名は1～80文字で入力してください"
        }
        val normalizedToken = invitationToken.trim()
        require(normalizedToken.length in 43..128) { "招待コードの形式が正しくありません" }

        return withContext(Dispatchers.IO) {
            val publicKey = security.prepare()
            val result = api.enroll(endpoint, normalizedToken, normalizedName, publicKey)
            val encryptedCredential = security.encryptCredential(result.credential)
            store.save(endpoint.toString(), result, encryptedCredential)
            SavedEnrollment(endpoint.toString(), result.deviceId, result.pairId, result.slot)
        }
    }

    suspend fun credential(): String? = withContext(Dispatchers.IO) {
        store.loadEncryptedCredential()?.let(security::decryptCredential)
    }
}
