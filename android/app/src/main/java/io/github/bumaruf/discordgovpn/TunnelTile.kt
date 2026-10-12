package io.github.bumaruf.discordgovpn

import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import androidx.core.content.ContextCompat
import io.github.bumaruf.discordgovpn.mobile.Mobile

/** The Quick Settings toggle: the Android counterpart of the desktop tray icon. */
class TunnelTile : TileService() {
    override fun onStartListening() {
        val tile = qsTile ?: return
        val status = Tunnel.status.value
        tile.state = if (status != null) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        tile.label = getString(R.string.tile_label)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            tile.subtitle = status?.label ?: Mobile.stateLabel(Mobile.StateStopped)
        }
        tile.updateTile()
    }

    override fun onClick() {
        if (Tunnel.status.value != null) {
            startService(TunnelService.intent(this, TunnelService.ACTION_STOP))
            return
        }
        // The VPN consent and a missing token both need the screen.
        if (VpnService.prepare(this) != null || !Profiles.hasAny(this)) {
            openApp(connect = Profiles.hasAny(this))
            return
        }
        try {
            ContextCompat.startForegroundService(this, TunnelService.connectIntent(this, null))
        } catch (e: IllegalStateException) {
            // Android 12+ may refuse a foreground service started from here.
            openApp(connect = true)
        }
    }

    private fun openApp(connect: Boolean) {
        val intent = Intent(this, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            .putExtra(MainActivity.EXTRA_CONNECT, connect)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startActivityAndCollapse(
                PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT),
            )
        } else {
            @Suppress("DEPRECATION")
            startActivityAndCollapse(intent)
        }
    }

    companion object {
        fun refresh(context: Context) {
            TileService.requestListeningState(context, ComponentName(context, TunnelTile::class.java))
        }
    }
}
