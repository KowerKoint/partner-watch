package com.kowerkoint.partnerwatch.status

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationManager
import android.os.CancellationSignal
import androidx.core.content.ContextCompat
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull
import java.time.Instant
import kotlin.coroutines.resume

data class LocationResult(
    val status:String,
    val latitude:Double=0.0,
    val longitude:Double=0.0,
    val accuracyMeters:Double=0.0,
    val observedAt:Instant?=null,
    val source:String="",
)

class LocationCollector(private val context:Context) {
    suspend fun collect(enabled:Boolean,precise:Boolean):LocationResult {
        if(!enabled)return LocationResult("DISABLED")
        val coarse=ContextCompat.checkSelfPermission(context,Manifest.permission.ACCESS_COARSE_LOCATION)==PackageManager.PERMISSION_GRANTED
        val fine=ContextCompat.checkSelfPermission(context,Manifest.permission.ACCESS_FINE_LOCATION)==PackageManager.PERMISSION_GRANTED
        val background=ContextCompat.checkSelfPermission(context,Manifest.permission.ACCESS_BACKGROUND_LOCATION)==PackageManager.PERMISSION_GRANTED
        if(!coarse||!background||(precise&&!fine))return LocationResult("PERMISSION_DENIED")
        val manager=context.getSystemService(LocationManager::class.java)
        if(!manager.isLocationEnabled)return LocationResult("SETTING_OFF")
        val providers=manager.getProviders(true).let { available ->
            listOf(LocationManager.FUSED_PROVIDER,if(precise)LocationManager.GPS_PROVIDER else LocationManager.NETWORK_PROVIDER)+available
        }.distinct().filter { runCatching{manager.isProviderEnabled(it)}.getOrDefault(false) }
        val fresh=providers.firstOrNull()?.let { current(manager,it) }
        if(fresh!=null)return fresh.toResult("FRESH")
        val cutoff=System.currentTimeMillis()-15*60*1000L
        val last=providers.mapNotNull{runCatching{manager.getLastKnownLocation(it)}.getOrNull()}.filter{it.time>=cutoff}.maxByOrNull{it.time}
        return last?.toResult("LAST_KNOWN")?:LocationResult("TIMEOUT")
    }

    private suspend fun current(manager:LocationManager,provider:String):Location?=withTimeoutOrNull(20_000){
        suspendCancellableCoroutine { continuation ->
            val cancellation=CancellationSignal();continuation.invokeOnCancellation{cancellation.cancel()}
            runCatching { manager.getCurrentLocation(provider,cancellation,context.mainExecutor){if(continuation.isActive)continuation.resume(it)} }
                .onFailure { if(continuation.isActive)continuation.resume(null) }
        }
    }
    private fun Location.toResult(source:String)=LocationResult("AVAILABLE",latitude,longitude,accuracy.toDouble(),Instant.ofEpochMilli(time),source)
}
