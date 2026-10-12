package io.github.bumaruf.discordgovpn.ui

import android.content.ClipboardManager
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import io.github.bumaruf.discordgovpn.R

/** The panel's BrandMark: the shield on a light rounded square. */
@Composable
fun BrandMark(size: Dp = 28.dp) {
    Box(
        Modifier.size(size).clip(Tokens.radius).background(Tokens.foreground),
        contentAlignment = Alignment.Center,
    ) {
        Icon(
            painterResource(R.drawable.ic_shield),
            contentDescription = null,
            tint = Tokens.background,
            modifier = Modifier.size(size * 4 / 7),
        )
    }
}

@Composable
fun BrandHeader() {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
        BrandMark()
        Text(stringResource(R.string.app_name), style = Type.brand, color = Tokens.foreground)
    }
}

/** shadcn's Card: card background, a faint ring, radius-xl. */
@Composable
fun PanelCard(
    modifier: Modifier = Modifier,
    spacing: Dp = 18.dp,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(
        modifier
            .fillMaxWidth()
            .clip(Tokens.radiusXl)
            .background(Tokens.card)
            .border(1.dp, Tokens.cardRing, Tokens.radiusXl)
            .padding(20.dp),
        verticalArrangement = Arrangement.spacedBy(spacing),
        content = content,
    )
}

@Composable
fun Divider() {
    Box(Modifier.fillMaxWidth().height(1.dp).background(Tokens.border))
}

/** The panel's StatusDot, with the halo it shows while live. */
@Composable
fun StatusDot(color: Color, halo: Boolean = false) {
    Box(Modifier.size(18.dp), contentAlignment = Alignment.Center) {
        if (halo) Box(Modifier.size(18.dp).clip(CircleShape).background(color.copy(alpha = 0.25f)))
        Box(Modifier.size(10.dp).clip(CircleShape).background(color))
    }
}

@Composable
fun Spinner() {
    CircularProgressIndicator(
        modifier = Modifier.size(18.dp),
        color = Tokens.foreground,
        trackColor = Tokens.input,
        strokeWidth = 3.dp,
    )
}

/** A label/value row, like the desktop screen's. */
@Composable
fun InfoRow(label: String, value: String, color: Color = Tokens.foreground, mono: Boolean = false) {
    Row {
        Text(label, style = Type.body, color = Tokens.mutedForeground, modifier = Modifier.width(88.dp))
        Text(value, style = if (mono) Type.mono else Type.body, color = color)
    }
}

private val buttonPadding = PaddingValues(horizontal = 14.dp)

@Composable
fun PrimaryButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Button(
        onClick = onClick,
        modifier = modifier.fillMaxWidth().height(48.dp),
        shape = Tokens.radius,
        colors = ButtonDefaults.buttonColors(
            containerColor = Tokens.foreground,
            contentColor = Tokens.primaryForeground,
        ),
    ) { Text(text, style = Type.button) }
}

@Composable
fun OutlineButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    OutlinedButton(
        onClick = onClick,
        modifier = modifier.fillMaxWidth().height(48.dp),
        shape = Tokens.radius,
        border = BorderStroke(1.dp, Tokens.input),
        colors = ButtonDefaults.outlinedButtonColors(contentColor = Tokens.foreground),
    ) { Text(text, style = Type.button) }
}

@Composable
fun SecondaryButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier) {
    Button(
        onClick = onClick,
        modifier = modifier.height(44.dp),
        shape = Tokens.radius,
        border = BorderStroke(1.dp, Tokens.input),
        contentPadding = buttonPadding,
        colors = ButtonDefaults.buttonColors(
            containerColor = Tokens.secondary,
            contentColor = Tokens.foreground,
        ),
    ) { Text(text, style = Type.label) }
}

/**
 * The token field with its paste button. Whitespace is dropped as it is typed, as on the
 * desktop: a pasted token often brings a line break, and none belongs inside one.
 */
@Composable
fun TokenField(value: String, onValueChange: (String) -> Unit, onDone: () -> Unit, label: String) {
    val context = LocalContext.current
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
        BasicTextField(
            value = value,
            onValueChange = { onValueChange(it.filterNot(Char::isWhitespace)) },
            singleLine = true,
            textStyle = Type.mono.copy(color = Tokens.foreground),
            cursorBrush = SolidColor(Tokens.foreground),
            keyboardOptions = KeyboardOptions(
                autoCorrectEnabled = false,
                keyboardType = KeyboardType.Ascii,
                imeAction = ImeAction.Done,
            ),
            keyboardActions = KeyboardActions(onDone = { onDone() }),
            modifier = Modifier
                .weight(1f)
                .height(44.dp)
                .semantics { contentDescription = label },
            decorationBox = { inner ->
                Box(
                    Modifier
                        .border(1.dp, Tokens.input, Tokens.radius)
                        .padding(horizontal = 12.dp),
                    contentAlignment = Alignment.CenterStart,
                ) {
                    if (value.isEmpty()) {
                        Text(stringResource(R.string.token_placeholder), style = Type.mono, color = Tokens.mutedForeground)
                    }
                    inner()
                }
            },
        )
        SecondaryButton(stringResource(R.string.paste), onClick = {
            val clip = context.getSystemService(ClipboardManager::class.java).primaryClip
            val text = clip?.takeIf { it.itemCount > 0 }?.getItemAt(0)?.coerceToText(context)?.toString()
            if (text != null) onValueChange(text.filterNot(Char::isWhitespace))
        })
    }
}

@Composable
fun InfoNote(text: String) {
    Row(
        Modifier
            .fillMaxWidth()
            .clip(Tokens.radiusXl)
            .background(Tokens.card)
            .border(1.dp, Tokens.cardRing, Tokens.radiusXl)
            .padding(horizontal = 16.dp, vertical = 14.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Icon(
            painterResource(R.drawable.ic_info),
            contentDescription = null,
            tint = Tokens.mutedForeground,
            modifier = Modifier.padding(top = 2.dp).size(16.dp),
        )
        Text(text, style = Type.small, color = Tokens.mutedForeground)
    }
}
