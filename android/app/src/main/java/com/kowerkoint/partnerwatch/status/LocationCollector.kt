package com.kowerkoint.partnerwatch.status

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationManager
import android.os.CancellationSignal
import android.os.SystemClock
import android.util.Log
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
        Log.d(TAG,"location providers: ${providers.joinToString()}")
        val fresh=providers.firstOrNull()?.let { current(manager,it) }
        if(fresh!=null){Log.d(TAG,"fresh location received from ${fresh.provider}");return fresh.toResult("FRESH")}
        val candidates=providers.mapNotNull{provider->runCatching{manager.getLastKnownLocation(provider)}.getOrNull()?.also{Log.d(TAG,"cached location from $provider age=${it.ageMillis()}ms")}}
        val last=candidates.filter{it.ageMillis()<=15*60*1000L}.minByOrNull{it.ageMillis()}
        Log.d(TAG,if(last==null)"no usable cached location" else "using cached location from ${last.provider}")
        return last?.toResult("LAST_KNOWN")?:LocationResult("TIMEOUT")
    }

    private suspend fun current(manager:LocationManager,provider:String):Location?=withTimeoutOrNull(20_000){
        suspendCancellableCoroutine { continuation ->
            val cancellation=CancellationSignal();continuation.invokeOnCancellation{cancellation.cancel()}
            runCatching { manager.getCurrentLocation(provider,cancellation,context.mainExecutor){if(continuation.isActive)continuation.resume(it)} }
                .onFailure { if(continuation.isActive)continuation.resume(null) }
        }
    }
    private fun Location.ageMillis()=((SystemClock.elapsedRealtimeNanos()-elapsedRealtimeNanos)/1_000_000L).coerceAtLeast(0)
    private fun Location.toResult(source:String)=LocationResult("AVAILABLE",latitude,longitude,accuracy.toDouble(),Instant.now().minusMillis(ageMillis()),source)
    private companion object { const val TAG="PartnerWatchLocation" }
}
