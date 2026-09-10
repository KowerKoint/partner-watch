package com.kowerkoint.partnerwatch.notification

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.notificationForwardingDataStore by preferencesDataStore(name = "notification_forwarding_settings")

class NotificationForwardingPreferences(private val context: Context) {
    val enabled: Flow<Boolean> = context.notificationForwardingDataStore.data.map { it[ENABLED] ?: false }

    suspend fun setEnabled(value: Boolean) {
        context.notificationForwardingDataStore.edit { it[ENABLED] = value }
    }

    private companion object {
        val ENABLED = booleanPreferencesKey("forward_notifications")
    }
}
