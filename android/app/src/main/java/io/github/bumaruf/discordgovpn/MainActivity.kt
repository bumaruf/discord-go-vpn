package io.github.bumaruf.discordgovpn

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import io.github.bumaruf.discordgovpn.ui.AppTheme
import io.github.bumaruf.discordgovpn.ui.MainScreen
import io.github.bumaruf.discordgovpn.ui.ScreenActions
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity(), ScreenActions {
    private var pendingToken: String? = null

    private val vpnConsent = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        if (result.resultCode == RESULT_OK) {
            startTunnel(pendingToken)
        } else {
            Tunnel.reportConnectError(getString(R.string.vpn_denied))
        }
        pendingToken = null
    }

    // Without it the notification is hidden, but the tunnel works the same.
    private val notificationPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) {}

    private val confPicker = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri: Uri? ->
        if (uri == null) return@registerForActivityResult
        if (Profiles.importConf(this, uri)) {
            connect(null)
        } else {
            Tunnel.reportConnectError(getString(R.string.conf_unreadable))
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge(
            statusBarStyle = SystemBarStyle.dark(android.graphics.Color.BLACK),
            navigationBarStyle = SystemBarStyle.dark(android.graphics.Color.BLACK),
        )
        super.onCreate(savedInstanceState)
        setContent { AppTheme { MainScreen(this) } }

        // Like the desktop reopening Discord in each new connection, but only while
        // this screen shows: Android doesn't let an app in the background open another.
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.RESUMED) {
                Tunnel.discordRequests.collect { openDiscord() }
            }
        }

        if (savedInstanceState == null) handleIntent(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        if (intent?.getBooleanExtra(EXTRA_CONNECT, false) == true && Tunnel.status.value == null) {
            connect(null)
        }
    }

    override fun connect(token: String?) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
        val consent = VpnService.prepare(this)
        if (consent != null) {
            pendingToken = token
            vpnConsent.launch(consent)
        } else {
            startTunnel(token)
        }
    }

    private fun startTunnel(token: String?) {
        ContextCompat.startForegroundService(this, TunnelService.connectIntent(this, token))
    }

    override fun disconnect() {
        startService(TunnelService.intent(this, TunnelService.ACTION_STOP))
    }

    override fun reconnect() {
        startService(TunnelService.intent(this, TunnelService.ACTION_RECONNECT))
    }

    override fun useToken(typed: String): String? = Tunnel.useToken(typed)

    override fun openDiscord() {
        packageManager.getLaunchIntentForPackage(TunnelService.DISCORD_PACKAGE)?.let { startActivity(it) }
    }

    override fun pickConf() {
        // A .conf has no registered MIME type.
        confPicker.launch(arrayOf("*/*"))
    }

    override fun changeToken() {
        Profiles.forgetAll(this)
        Tunnel.reportConnectError(null)
    }

    override fun hasProfile(): Boolean = Profiles.hasAny(this)

    companion object {
        const val EXTRA_CONNECT = "connect"
    }
}
