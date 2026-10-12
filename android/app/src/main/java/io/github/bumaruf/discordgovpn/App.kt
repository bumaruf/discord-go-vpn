package io.github.bumaruf.discordgovpn

import android.app.Application
import android.app.NotificationChannel
import android.app.NotificationManager
import io.github.bumaruf.discordgovpn.mobile.Mobile

class App : Application() {
    override fun onCreate() {
        super.onCreate()
        Mobile.init(filesDir.path)

        val channel = NotificationChannel(
            TunnelService.CHANNEL_ID,
            getString(R.string.notification_channel),
            NotificationManager.IMPORTANCE_LOW,
        )
        getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }
}
