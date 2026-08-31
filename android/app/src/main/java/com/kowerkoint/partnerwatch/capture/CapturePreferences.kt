package com.kowerkoint.partnerwatch.capture

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.captureDataStore by preferencesDataStore(name = "capture_settings")

class CapturePreferences(private val context: Context) {
    val accepting: Flow<Boolean> = context.captureDataStore.data.map { values ->
        values[ACCEPTING] ?: false
    }

    suspend fun setAccepting(value: Boolean) {
        context.captureDataStore.edit { it[ACCEPTING] = value }
    }

    private companion object {
        val ACCEPTING = booleanPreferencesKey("accepting_capture_requests")
    }
}
