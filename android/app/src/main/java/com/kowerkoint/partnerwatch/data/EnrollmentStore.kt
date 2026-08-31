package com.kowerkoint.partnerwatch.data

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first

private val Context.enrollmentDataStore by preferencesDataStore(name = "enrollment")

data class SavedEnrollment(
    val serverUrl: String,
    val deviceId: String,
    val pairId: String,
    val slot: Int,
)

class EnrollmentStore(private val context: Context) {
    suspend fun load(): SavedEnrollment? {
        val values = context.enrollmentDataStore.data.first()
        val serverUrl = values[SERVER_URL] ?: return null
        val deviceId = values[DEVICE_ID] ?: return null
        val pairId = values[PAIR_ID] ?: return null
        val slot = values[SLOT] ?: return null
        if (values[ENCRYPTED_CREDENTIAL].isNullOrBlank()) return null
        return SavedEnrollment(serverUrl, deviceId, pairId, slot)
    }

    suspend fun loadEncryptedCredential(): String? =
        context.enrollmentDataStore.data.first()[ENCRYPTED_CREDENTIAL]

    suspend fun save(
        serverUrl: String,
        result: EnrollmentResult,
        encryptedCredential: String,
    ) {
        context.enrollmentDataStore.edit { values ->
            values[SERVER_URL] = serverUrl
            values[DEVICE_ID] = result.deviceId
            values[PAIR_ID] = result.pairId
            values[SLOT] = result.slot
            values[ENCRYPTED_CREDENTIAL] = encryptedCredential
        }
    }

    private companion object {
        val SERVER_URL = stringPreferencesKey("server_url")
        val DEVICE_ID = stringPreferencesKey("device_id")
        val PAIR_ID = stringPreferencesKey("pair_id")
        val SLOT = intPreferencesKey("slot")
        val ENCRYPTED_CREDENTIAL = stringPreferencesKey("encrypted_credential")
    }
}
