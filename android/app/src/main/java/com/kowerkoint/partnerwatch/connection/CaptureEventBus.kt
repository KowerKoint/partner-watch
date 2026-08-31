package com.kowerkoint.partnerwatch.connection

import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow

data class CaptureCompletedEvent(
    val requestId: String,
    val status: String,
    val imageId: String,
    val failure: String,
)

object CaptureEventBus {
    private val mutableEvents = MutableSharedFlow<CaptureCompletedEvent>(extraBufferCapacity = 8)
    val events = mutableEvents.asSharedFlow()
    fun publish(event: CaptureCompletedEvent) { mutableEvents.tryEmit(event) }
}
