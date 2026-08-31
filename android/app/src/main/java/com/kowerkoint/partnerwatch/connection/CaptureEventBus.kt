package com.kowerkoint.partnerwatch.connection

import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow

data class CaptureCompletedEvent(
    val requestId: String,
    val status: String,
    val imageId: String,
    val failure: String,
)

enum class ConnectionStatus { STARTING, CONNECTED, RECONNECTING }

object ConnectionStatusBus {
    private val mutableStatus = MutableStateFlow(ConnectionStatus.STARTING)
    val status = mutableStatus.asStateFlow()
    fun set(value: ConnectionStatus) { mutableStatus.value = value }
}

object CaptureEventBus {
    private val mutableEvents = MutableSharedFlow<CaptureCompletedEvent>(extraBufferCapacity = 8)
    val events = mutableEvents.asSharedFlow()
    fun publish(event: CaptureCompletedEvent) { mutableEvents.tryEmit(event) }
}
