package io.github.bumaruf.discordgovpn

import android.content.Context
import android.net.Uri
import android.os.Build
import android.provider.OpenableColumns
import android.provider.Settings
import io.github.bumaruf.discordgovpn.mobile.Mobile
import java.io.File

/**
 * Where access comes from, as on the desktop: a .conf wins when there is one, otherwise
 * the saved connection token, which the Go side keeps in the app's files.
 */
object Profiles {
    private fun dir(context: Context) = File(context.filesDir, "profiles")

    fun conf(context: Context): File? =
        dir(context).listFiles { f -> f.name.endsWith(".conf") }?.firstOrNull()

    fun hasAny(context: Context) = conf(context) != null || Mobile.savedToken().isNotEmpty()

    /** Copies a picked .conf in, replacing any other; false when it can't be read. */
    fun importConf(context: Context, uri: Uri): Boolean {
        val name = displayName(context, uri)
            ?.replace(Regex("[^A-Za-z0-9._-]"), "_")
            ?.let { if (it.endsWith(".conf")) it else "$it.conf" }
            ?: "perfil.conf"
        val bytes = try {
            context.contentResolver.openInputStream(uri)?.use { it.readBytes() }
        } catch (e: Exception) {
            null
        } ?: return false

        forgetConf(context)
        val dir = dir(context).apply { mkdirs() }
        File(dir, name).writeBytes(bytes)
        return true
    }

    fun forgetConf(context: Context) {
        dir(context).deleteRecursively()
    }

    fun forgetAll(context: Context) {
        forgetConf(context)
        Mobile.forgetToken()
    }

    /** The phone's name in the panel: the one set in Settings, else the model. */
    fun deviceLabel(context: Context): String =
        Settings.Global.getString(context.contentResolver, Settings.Global.DEVICE_NAME)
            ?.takeIf { it.isNotBlank() }
            ?: Build.MODEL

    private fun displayName(context: Context, uri: Uri): String? =
        context.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)
            ?.use { if (it.moveToFirst()) it.getString(0) else null }
}
