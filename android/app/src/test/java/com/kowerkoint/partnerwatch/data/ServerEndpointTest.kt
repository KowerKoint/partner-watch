package com.kowerkoint.partnerwatch.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ServerEndpointTest {
    @Test
    fun acceptsHttpsOrigin() {
        assertEquals(
            "https://watch.example.com/",
            ServerEndpoint.parse(" https://watch.example.com ").toString(),
        )
    }

    @Test
    fun rejectsHttpAndUrlWithSecretsOrExtraComponents() {
        assertNull(ServerEndpoint.parse("http://watch.example.com"))
        assertNull(ServerEndpoint.parse("https://user:pass@watch.example.com"))
        assertNull(ServerEndpoint.parse("https://watch.example.com/path"))
        assertNull(ServerEndpoint.parse("https://watch.example.com/?token=secret"))
        assertNull(ServerEndpoint.parse("https://watch.example.com/#fragment"))
    }
}
