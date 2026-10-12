package io.github.bumaruf.discordgovpn

import io.github.bumaruf.discordgovpn.mobile.Mobile
import io.github.bumaruf.discordgovpn.mobile.Session
import io.github.bumaruf.discordgovpn.mobile.Status
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow

/** A copy of the Go Status, so the screen never holds a reference into Go. */
data class TunnelStatus(
    val state: Long,
    val label: String,
    val attempt: Long,
    val since: Long,
    val discordTunneled: Boolean,
    val lastError: String,
    val lastErrorAt: Long,
    val progress: String,
    val warning: String,
    val tokenAsk: Long,
    val tokenAskReason: String,
    val profile: String,
    val server: String,
) {
    companion object {
        fun from(s: Status) = TunnelStatus(
            state = s.state,
            label = s.label,
            attempt = s.attempt,
            since = s.since,
            discordTunneled = s.discordTunneled,
            lastError = s.lastError,
            lastErrorAt = s.lastErrorAt,
            progress = s.progress,
            warning = s.warning,
            tokenAsk = s.tokenAsk,
            tokenAskReason = s.tokenAskReason,
            profile = s.profile,
            server = s.server,
        )

        /** Shown while Connect validates the token, before the supervisor reports. */
        fun starting() = TunnelStatus(
            state = Mobile.StateStarting,
            label = Mobile.stateLabel(Mobile.StateStarting),
            attempt = 0,
            since = System.currentTimeMillis(),
            discordTunneled = false,
            lastError = "",
            lastErrorAt = 0,
            progress = "",
            warning = "",
            tokenAsk = Mobile.TokenFine,
            tokenAskReason = "",
            profile = "",
            server = "",
        )
    }
}

/**
 * The tunnel as the rest of the app sees it. TunnelService is its only writer; the
 * screen, the tile and the notification only read it, like the desktop UI reads the
 * supervisor's snapshots.
 */
object Tunnel {
    private val _status = MutableStateFlow<TunnelStatus?>(null)

    /** null while no session runs. */
    val status: StateFlow<TunnelStatus?> = _status

    private val _connectError = MutableStateFlow<String?>(null)

    /** Why the last Connect failed, such as a refused token. */
    val connectError: StateFlow<String?> = _connectError

    private val _discordRequests = MutableSharedFlow<Unit>(extraBufferCapacity = 1)

    /** A connection came up; the screen opens Discord if it is showing. */
    val discordRequests: SharedFlow<Unit> = _discordRequests

    @Volatile
    var session: Session? = null
        internal set

    internal fun publish(status: TunnelStatus?) {
        _status.value = status
    }

    fun reportConnectError(message: String?) {
        _connectError.value = message
    }

    internal fun requestDiscord() {
        _discordRequests.tryEmit(Unit)
    }

    /** Hands a pasted token to the running session; returns why it was rejected. */
    fun useToken(typed: String): String? = try {
        session?.useToken(typed)
        null
    } catch (e: Exception) {
        e.message
    }
}
