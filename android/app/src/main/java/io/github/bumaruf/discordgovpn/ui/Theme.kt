package io.github.bumaruf.discordgovpn.ui

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.bumaruf.discordgovpn.R

/** The panel's tokens, from panel/app/globals.css: dark only. */
object Tokens {
    val background = Color(0xFF000000)
    val foreground = Color(0xFFEDEDED)
    val card = Color(0xFF0A0A0A)
    val primaryForeground = Color(0xFF0A0A0A)
    val secondary = Color(0xFF1A1A1A)
    val mutedForeground = Color(0xFFA1A1A1)
    val destructive = Color(0xFFFF6166)
    val border = Color(0xFF242424)
    val input = Color(0xFF2E2E2E)
    val ring = Color(0xFF52A8FF)
    val success = Color(0xFF45D483)
    val warning = Color(0xFFF5A623)

    /** The card ring, ring-foreground/10. */
    val cardRing = foreground.copy(alpha = 0.1f)

    /** --radius is 0.5rem; cards use radius-xl. */
    val radius = RoundedCornerShape(8.dp)
    val radiusXl = RoundedCornerShape(12.dp)
}

val Geist = FontFamily(
    Font(R.font.geist_regular, FontWeight.Normal),
    Font(R.font.geist_medium, FontWeight.Medium),
    Font(R.font.geist_semibold, FontWeight.SemiBold),
)

val GeistMono = FontFamily(Font(R.font.geist_mono_regular, FontWeight.Normal))

object Type {
    val title = TextStyle(fontFamily = Geist, fontWeight = FontWeight.SemiBold, fontSize = 28.sp, letterSpacing = (-0.56).sp)
    val heading = TextStyle(fontFamily = Geist, fontWeight = FontWeight.SemiBold, fontSize = 24.sp, letterSpacing = (-0.48).sp)
    val cardTitle = TextStyle(fontFamily = Geist, fontWeight = FontWeight.Medium, fontSize = 16.sp)
    val brand = TextStyle(fontFamily = Geist, fontWeight = FontWeight.SemiBold, fontSize = 15.sp, letterSpacing = (-0.15).sp)
    val body = TextStyle(fontFamily = Geist, fontSize = 14.sp, lineHeight = 21.sp)
    val small = TextStyle(fontFamily = Geist, fontSize = 13.sp, lineHeight = 19.sp)
    val label = TextStyle(fontFamily = Geist, fontWeight = FontWeight.Medium, fontSize = 14.sp)
    val button = TextStyle(fontFamily = Geist, fontWeight = FontWeight.Medium, fontSize = 15.sp)
    val mono = TextStyle(fontFamily = GeistMono, fontSize = 13.sp, lineHeight = 19.sp)
}

@Composable
fun AppTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = darkColorScheme(
            primary = Tokens.foreground,
            onPrimary = Tokens.primaryForeground,
            background = Tokens.background,
            onBackground = Tokens.foreground,
            surface = Tokens.card,
            onSurface = Tokens.foreground,
            error = Tokens.destructive,
            outline = Tokens.input,
        ),
        content = content,
    )
}
