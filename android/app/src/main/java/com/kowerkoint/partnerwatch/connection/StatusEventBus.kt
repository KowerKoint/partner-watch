package com.kowerkoint.partnerwatch.connection

import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow

data class StatusCompletedEvent(val requestId: String)

object StatusEventBus {
    private val mutableEvents = MutableSharedFlow<StatusCompletedEvent>(extraBufferCapacity = 16)
    val events = mutableEvents.asSharedFlow()
    fun publish(event: StatusCompletedEvent) { mutableEvents.tryEmit(event) }
}
