package com.kowerkoint.partnerwatch.status

import android.content.Context
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.first

private val Context.statusDataStore by preferencesDataStore(name = "status_settings")

class StatusPreferences(private val context: Context) {
    val sharingBattery: Flow<Boolean> = context.statusDataStore.data.map { it[BATTERY] ?: false }
    val sharingLocation: Flow<Boolean> = context.statusDataStore.data.map { it[LOCATION] ?: false }
    val preciseLocation: Flow<Boolean> = context.statusDataStore.data.map { it[PRECISE] ?: false }
    suspend fun setSharingBattery(value: Boolean) { context.statusDataStore.edit { it[BATTERY] = value } }
    suspend fun isSharingBattery(): Boolean = sharingBattery.first()
    suspend fun setSharingLocation(value:Boolean){context.statusDataStore.edit{it[LOCATION]=value}}
    suspend fun setPreciseLocation(value:Boolean){context.statusDataStore.edit{it[PRECISE]=value}}
    suspend fun isSharingLocation()=sharingLocation.first()
    suspend fun isPreciseLocation()=preciseLocation.first()
    private companion object { val BATTERY = booleanPreferencesKey("sharing_battery");val LOCATION=booleanPreferencesKey("sharing_location");val PRECISE=booleanPreferencesKey("precise_location") }
}
