package com.kowerkoint.partnerwatch.connection

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

enum class ConnectionMode { ALWAYS_CONNECTED, FCM_ONLY }

private val Context.connectionDataStore by preferencesDataStore(name = "connection_settings")

class ConnectionPreferences(private val context: Context) {
    val mode: Flow<ConnectionMode> = context.connectionDataStore.data.map { values ->
        when (values[MODE]) { "FCM_ONLY" -> ConnectionMode.FCM_ONLY; else -> ConnectionMode.ALWAYS_CONNECTED }
    }
    suspend fun setMode(mode: ConnectionMode) { context.connectionDataStore.edit { it[MODE] = mode.name } }
    private companion object { val MODE = stringPreferencesKey("connection_mode") }
}
