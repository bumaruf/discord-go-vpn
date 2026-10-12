package io.github.bumaruf.discordgovpn.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.bumaruf.discordgovpn.R
import io.github.bumaruf.discordgovpn.Tunnel
import io.github.bumaruf.discordgovpn.TunnelStatus
import io.github.bumaruf.discordgovpn.mobile.Mobile
import kotlinx.coroutines.delay
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/** What the screen asks of the activity. */
interface ScreenActions {
    /** token is a freshly typed one; null uses the saved token or .conf. */
    fun connect(token: String?)
    fun disconnect()
    fun reconnect()

    /** Returns why the token was rejected, or null. */
    fun useToken(typed: String): String?
    fun openDiscord()
    fun pickConf()
    fun changeToken()
    fun hasProfile(): Boolean
}

@Composable
fun MainScreen(actions: ScreenActions) {
    val status by Tunnel.status.collectAsStateWithLifecycle()
    val connectError by Tunnel.connectError.collectAsStateWithLifecycle()
    var forgotten by remember { mutableIntStateOf(0) }
    // Read again whenever a session ends or a connect fails: a refusal forgets the token.
    val hasProfile = remember(status == null, connectError, forgotten) { actions.hasProfile() }

    Box(
        Modifier
            .fillMaxSize()
            .background(Tokens.background)
            .windowInsetsPadding(WindowInsets.safeDrawing),
    ) {
        val current = status
        when {
            current != null -> StatusScreen(current, actions)
            hasProfile -> StoppedScreen(connectError, actions, onChangeToken = {
                actions.changeToken()
                forgotten++
            })
            else -> TokenScreen(connectError, actions)
        }
    }
}

/** A screen's frame: scrolling content above, the buttons pinned below. */
@Composable
private fun Screen(
    buttons: @Composable ColumnScope.() -> Unit,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(Modifier.fillMaxSize().imePadding().padding(start = 20.dp, end = 20.dp, top = 28.dp, bottom = 24.dp)) {
        Column(
            Modifier.weight(1f).fillMaxWidth().verticalScroll(rememberScrollState()),
            verticalArrangement = Arrangement.spacedBy(20.dp),
            content = content,
        )
        Spacer(Modifier.height(20.dp))
        Column(verticalArrangement = Arrangement.spacedBy(10.dp), content = buttons)
    }
}

@Composable
private fun TokenScreen(connectError: String?, actions: ScreenActions) {
    var typed by rememberSaveable { mutableStateOf("") }
    var localError by remember { mutableStateOf<String?>(null) }
    val submit = {
        localError = try {
            Mobile.checkToken(typed)
            actions.connect(typed)
            null
        } catch (e: Exception) {
            e.message
        }
    }
    val error = localError ?: connectError

    // Centered like the panel's login page, scrolling when the keyboard leaves no room.
    BoxWithConstraints(Modifier.fillMaxSize().imePadding()) {
        Column(
            Modifier
                .verticalScroll(rememberScrollState())
                .heightIn(min = maxHeight)
                .padding(start = 20.dp, end = 20.dp, top = 28.dp, bottom = 24.dp),
            verticalArrangement = Arrangement.SpaceBetween,
        ) {
            Spacer(Modifier.height(0.dp))
            Column(verticalArrangement = Arrangement.spacedBy(28.dp)) {
                Column(
                    Modifier.fillMaxWidth(),
                    horizontalAlignment = Alignment.CenterHorizontally,
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    BrandMark(40.dp)
                    Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        Text(stringResource(R.string.token_title), style = Type.heading, color = Tokens.foreground)
                        Text(
                            stringResource(R.string.token_intro),
                            style = Type.body,
                            color = Tokens.mutedForeground,
                            textAlign = TextAlign.Center,
                        )
                    }
                }

                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    val label = stringResource(R.string.token_label)
                    Text(label, style = Type.label, color = Tokens.foreground)
                    TokenField(typed, onValueChange = { typed = it; localError = null }, onDone = submit, label = label)
                    if (error != null) {
                        Text(error, style = Type.small, color = Tokens.destructive)
                    } else {
                        Text(stringResource(R.string.token_hint), style = Type.small, color = Tokens.mutedForeground)
                    }
                }

                PrimaryButton(stringResource(R.string.connect), onClick = submit)
            }

            Row(Modifier.fillMaxWidth().padding(top = 28.dp), horizontalArrangement = Arrangement.Center) {
                Text(stringResource(R.string.conf_prompt) + " ", style = Type.small, color = Tokens.mutedForeground)
                Text(
                    stringResource(R.string.conf_use),
                    style = Type.small.copy(fontWeight = FontWeight.Medium),
                    color = Tokens.foreground,
                    modifier = Modifier.clickable(role = Role.Button, onClick = actions::pickConf),
                )
            }
        }
    }
}

@Composable
private fun StoppedScreen(connectError: String?, actions: ScreenActions, onChangeToken: () -> Unit) {
    Screen(
        buttons = {
            PrimaryButton(stringResource(R.string.connect), onClick = { actions.connect(null) })
            OutlineButton(stringResource(R.string.change_token), onClick = onChangeToken)
        },
    ) {
        BrandHeader()
        PanelCard {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                StatusDot(Tokens.mutedForeground.copy(alpha = 0.4f))
                Text(Mobile.stateLabel(Mobile.StateStopped), style = Type.title, color = Tokens.foreground)
            }
            if (connectError != null) ErrorBox(connectError, at = 0, current = true)
        }
    }
}

@Composable
private fun StatusScreen(status: TunnelStatus, actions: ScreenActions) {
    val now = rememberNow()
    val elapsed = Mobile.duration(now - status.since)

    Screen(
        buttons = {
            when (status.state) {
                Mobile.StateStarting -> OutlineButton(stringResource(R.string.cancel), onClick = actions::disconnect)
                Mobile.StateHealthy -> {
                    PrimaryButton(stringResource(R.string.open_discord), onClick = actions::openDiscord)
                    OutlineButton(stringResource(R.string.disconnect), onClick = actions::disconnect)
                }
                else -> {
                    PrimaryButton(stringResource(R.string.retry_now), onClick = actions::reconnect)
                    OutlineButton(stringResource(R.string.disconnect), onClick = actions::disconnect)
                }
            }
        },
    ) {
        BrandHeader()

        PanelCard {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    when (status.state) {
                        Mobile.StateStarting -> Spinner()
                        Mobile.StateHealthy -> StatusDot(Tokens.success, halo = true)
                        Mobile.StateDegraded -> StatusDot(Tokens.warning)
                        else -> StatusDot(Tokens.destructive)
                    }
                    Text(status.label, style = Type.title, color = Tokens.foreground)
                }
                val detail = when (status.state) {
                    Mobile.StateHealthy -> stringResource(R.string.since, elapsed)
                    Mobile.StateDegraded -> stringResource(R.string.attempt_since, status.attempt.toInt(), elapsed)
                    Mobile.StateFallback -> stringResource(R.string.fallback_detail)
                    else -> status.progress
                }
                if (detail.isNotBlank()) Text(detail, style = Type.body, color = Tokens.mutedForeground)
            }

            if (status.profile.isNotBlank()) {
                Divider()
                Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    InfoRow(stringResource(R.string.row_profile), status.profile)
                    if (status.server.isNotBlank()) InfoRow(stringResource(R.string.row_server), status.server, mono = true)
                    DiscordRow(status)
                }
            }

            if (status.warning.isNotBlank()) Text(status.warning, style = Type.small, color = Tokens.warning)
            if (status.lastError.isNotBlank()) {
                ErrorBox(status.lastError, status.lastErrorAt, current = status.state != Mobile.StateHealthy)
            }
        }

        if (status.tokenAsk != Mobile.TokenFine) TokenAskCard(status.tokenAskReason, actions)
        if (status.state == Mobile.StateHealthy) InfoNote(stringResource(R.string.only_discord))
    }
}

/** As on the desktop: inside only while a tunnel is up, outside once handed back. */
@Composable
private fun DiscordRow(status: TunnelStatus) {
    val label = stringResource(R.string.row_discord)
    when {
        status.state == Mobile.StateHealthy && status.discordTunneled ->
            InfoRow(label, stringResource(R.string.discord_inside), Tokens.success)
        status.state == Mobile.StateFallback ->
            InfoRow(label, stringResource(R.string.discord_outside), Tokens.destructive)
        else -> InfoRow(label, stringResource(R.string.discord_waiting), Tokens.mutedForeground)
    }
}

/**
 * The last failure, with its time so an old error doesn't read as a current one. Once
 * the tunnel is back up it stays, dimmed, as the desktop screen keeps it.
 */
@Composable
private fun ErrorBox(message: String, at: Long, current: Boolean) {
    val color = if (current) Tokens.destructive else Tokens.mutedForeground
    Row(
        Modifier
            .fillMaxWidth()
            .clip(Tokens.radius)
            .background(if (current) Tokens.destructive.copy(alpha = 0.08f) else Tokens.secondary)
            .border(1.dp, if (current) Tokens.destructive.copy(alpha = 0.25f) else Tokens.border, Tokens.radius)
            .padding(horizontal = 14.dp, vertical = 12.dp),
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        if (at > 0) Text(clock(at), style = Type.mono, color = Tokens.mutedForeground)
        Text(message, style = Type.small, color = color)
    }
}

@Composable
private fun TokenAskCard(reason: String, actions: ScreenActions) {
    var typed by rememberSaveable { mutableStateOf("") }
    var error by remember { mutableStateOf<String?>(null) }
    val submit = {
        error = actions.useToken(typed)
        if (error == null) typed = ""
    }

    PanelCard(spacing = 12.dp) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(stringResource(R.string.token_new_title), style = Type.cardTitle, color = Tokens.foreground)
            Text(reason, style = Type.small, color = Tokens.mutedForeground)
        }
        TokenField(typed, onValueChange = { typed = it; error = null }, onDone = submit, label = stringResource(R.string.token_label))
        SecondaryButton(stringResource(R.string.token_use), onClick = submit, modifier = Modifier.fillMaxWidth())
        error?.let { Text(it, style = Type.small, color = Tokens.destructive) }
    }
}

/** The current time, ticking once a second so elapsed times move. */
@Composable
private fun rememberNow(): Long {
    val now by produceState(System.currentTimeMillis()) {
        while (true) {
            delay(1000)
            value = System.currentTimeMillis()
        }
    }
    return now
}

private val clockFormat = DateTimeFormatter.ofPattern("HH:mm")

private fun clock(ms: Long): String = clockFormat.format(Instant.ofEpochMilli(ms).atZone(ZoneId.systemDefault()))
