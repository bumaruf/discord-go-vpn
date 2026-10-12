package io.github.bumaruf.discordgovpn

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import io.github.bumaruf.discordgovpn.mobile.Host
import io.github.bumaruf.discordgovpn.mobile.Mobile
import io.github.bumaruf.discordgovpn.mobile.Status
import java.util.concurrent.Executors

/**
 * Owns the VPN interface and the Go session. Connect and Stop block on the network, so
 * they run in order on one worker thread; Go reports back through [Host] from its own.
 */
class TunnelService : VpnService(), Host {
    private val worker = Executors.newSingleThreadExecutor()

    /** Set once a stop is asked for, so a late update can't bring the screen back. */
    @Volatile
    private var stopping = false

    @Volatile
    private var connecting = false

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_RECONNECT -> Tunnel.session?.reconnect()
            ACTION_STOP -> stop()
            // ACTION_CONNECT, or the system starting an always-on VPN.
            else -> start(intent?.getStringExtra(EXTRA_TOKEN))
        }
        return START_NOT_STICKY
    }

    private fun start(token: String?) {
        if (Tunnel.session != null || connecting) return
        connecting = true
        stopping = false

        val starting = TunnelStatus.starting()
        Tunnel.reportConnectError(null)
        Tunnel.publish(starting)
        ServiceCompat.startForeground(
            this,
            NOTIFICATION_ID,
            notification(starting),
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE
            } else {
                0
            },
        )
        TunnelTile.refresh(this)

        worker.execute {
            val conf = Profiles.conf(this)
            try {
                Tunnel.session = if (conf != null) {
                    Mobile.connectFile(conf.path, this)
                } else {
                    Mobile.connect(token ?: Mobile.savedToken(), Profiles.deviceLabel(this), this)
                }
            } catch (e: Exception) {
                // A .conf that fails to open fails the same way every time.
                if (conf != null) Profiles.forgetConf(this)
                Tunnel.reportConnectError(e.message)
                finish()
            } finally {
                connecting = false
            }
        }
    }

    private fun stop() {
        stopping = true
        Tunnel.publish(null)
        worker.execute {
            Tunnel.session?.stop()
            Tunnel.session = null
            finish()
        }
    }

    private fun finish() {
        Tunnel.publish(null)
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        stopSelf()
        TunnelTile.refresh(this)
    }

    /** The user took the permission away or turned on another VPN. */
    override fun onRevoke() {
        stop()
    }

    override fun onDestroy() {
        if (Tunnel.session != null) stop()
        worker.shutdown()
        super.onDestroy()
    }

    override fun establish(addresses: String, routes: String, dns: String, mtu: Long): Long {
        val builder = Builder()
            .setSession(getString(R.string.app_name))
            .setMtu(mtu.toInt())
        for (address in addresses.split(",")) {
            val (ip, prefix) = address.split("/")
            builder.addAddress(ip, prefix.toInt())
        }
        for (route in routes.split(",")) {
            val (ip, prefix) = route.split("/")
            builder.addRoute(ip, prefix.toInt())
        }
        for (server in dns.split(",")) {
            builder.addDnsServer(server)
        }
        try {
            builder.addAllowedApplication(DISCORD_PACKAGE)
        } catch (e: PackageManager.NameNotFoundException) {
            throw Exception(getString(R.string.discord_missing))
        }
        val tun = builder.establish() ?: throw Exception(getString(R.string.vpn_not_granted))
        return tun.detachFd().toLong()
    }

    override fun openDiscord() {
        Tunnel.requestDiscord()
    }

    override fun update(status: Status) {
        if (stopping) return
        val snapshot = TunnelStatus.from(status)
        // The last frame of a session that ends on its own: finish() clears it.
        if (snapshot.state == Mobile.StateStopped) return
        Tunnel.publish(snapshot)
        getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, notification(snapshot))
        TunnelTile.refresh(this)
    }

    private fun notification(s: TunnelStatus): Notification {
        val text = when (s.state) {
            Mobile.StateHealthy -> getString(R.string.notification_healthy)
            Mobile.StateDegraded -> listOf(getString(R.string.notification_attempt, s.attempt.toInt()), s.lastError)
                .filter { it.isNotBlank() }
                .joinToString(" · ")
            Mobile.StateFallback -> getString(R.string.fallback_detail)
            else -> s.progress.ifBlank { s.profile }
        }

        val builder = NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_shield)
            .setContentTitle(s.label)
            .setContentText(text)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setSilent(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .setContentIntent(
                PendingIntent.getActivity(
                    this, 0,
                    Intent(this, MainActivity::class.java),
                    PendingIntent.FLAG_IMMUTABLE,
                ),
            )
        if (s.state == Mobile.StateHealthy) {
            builder.setWhen(s.since).setShowWhen(true).setUsesChronometer(true)
        }
        if (s.state == Mobile.StateDegraded || s.state == Mobile.StateFallback) {
            builder.addAction(0, getString(R.string.retry_now), serviceAction(ACTION_RECONNECT))
        }
        builder.addAction(0, getString(R.string.disconnect), serviceAction(ACTION_STOP))
        return builder.build()
    }

    private fun serviceAction(action: String) = PendingIntent.getService(
        this, action.hashCode(), intent(this, action), PendingIntent.FLAG_IMMUTABLE,
    )

    companion object {
        const val CHANNEL_ID = "tunnel"
        const val DISCORD_PACKAGE = "com.discord"

        const val ACTION_CONNECT = "io.github.bumaruf.discordgovpn.CONNECT"
        const val ACTION_RECONNECT = "io.github.bumaruf.discordgovpn.RECONNECT"
        const val ACTION_STOP = "io.github.bumaruf.discordgovpn.STOP"
        private const val EXTRA_TOKEN = "token"
        private const val NOTIFICATION_ID = 1

        fun intent(context: Context, action: String): Intent =
            Intent(context, TunnelService::class.java).setAction(action)

        /** token is a freshly typed one; null leases with the saved token or the .conf. */
        fun connectIntent(context: Context, token: String?): Intent =
            intent(context, ACTION_CONNECT).apply { token?.let { putExtra(EXTRA_TOKEN, it) } }
    }
}
