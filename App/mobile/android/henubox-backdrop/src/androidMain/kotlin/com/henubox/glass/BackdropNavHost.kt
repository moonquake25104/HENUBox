package com.henubox.glass

import android.app.Activity
import android.graphics.Paint
import android.os.Build
import android.view.View
import android.view.ViewGroup
import android.webkit.WebView
import android.widget.FrameLayout
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.graphics.nativeCanvas
import androidx.compose.ui.platform.ComposeView
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.ViewCompositionStrategy
import androidx.compose.ui.layout.LayoutCoordinates
import androidx.compose.ui.layout.positionInWindow
import androidx.compose.ui.graphics.GraphicsLayerScope
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.core.graphics.PathParser
import com.kyant.backdrop.Backdrop
import com.kyant.backdrop.drawBackdrop
import com.kyant.backdrop.effects.blur
import com.kyant.backdrop.effects.lens
import com.kyant.backdrop.highlight.Highlight
import com.kyant.backdrop.shadow.Shadow
import java.util.concurrent.atomic.AtomicLong
import kotlin.math.roundToInt

/** One WebView draw into a shared compositing RenderNode; upstream Backdrop applies optics. */
class BackdropNavHost private constructor(
    private val activity: Activity,
    private val web: WebView,
    private val container: FrameLayout,
    private val original: View
) {
    private data class State(
        val enabled: Boolean = false, val x: Float = 0f, val y: Float = 0f,
        val width: Float = 0f, val height: Float = 0f, val ratio: Float = 1f,
        val selected: Int = 0, val position: Float = 0f, val pressed: Boolean = false,
        val dark: Boolean = false, val ink: Int = android.graphics.Color.BLACK,
        val reduced: Boolean = false
    )
    private var state by mutableStateOf(State())
    private var running by mutableStateOf(true)
    private var revision = -1L
    private var disposed = false
    private var frame by mutableStateOf(0L)
    private val source = object : FrameLayout(activity) {
        val layer = android.graphics.RenderNode("HENUBox existing WebView content").apply {
            setUseCompositingLayer(true,null)
            setClipToBounds(true)
        }
        override fun onDescendantInvalidated(child: View, target: View) {
            super.onDescendantInvalidated(child,target)
            // Chromium can update its child RenderNode without dirtying this
            // parent's cached display list. Re-record only on that invalidation.
            if(state.enabled && running) invalidate()
        }
        override fun dispatchDraw(canvas: android.graphics.Canvas) {
            if(state.enabled && running && canvas.isHardwareAccelerated) {
                layer.setPosition(0,0,width,height)
                val recording = layer.beginRecording(width,height)
                try { super.dispatchDraw(recording) } finally { layer.endRecording() }
                canvas.drawRenderNode(layer)
                backdropDraws.incrementAndGet(); frame++
            } else {
                super.dispatchDraw(canvas)
                layer.discardDisplayList()
            }
        }
    }
    val backdropDraws = AtomicLong()
    val navDraws = AtomicLong()
    private val compose = ComposeView(activity).apply {
        setViewCompositionStrategy(ViewCompositionStrategy.DisposeOnViewTreeLifecycleDestroyed)
        setBackgroundColor(android.graphics.Color.TRANSPARENT)
    }
    init {
        val index = container.indexOfChild(original)
        container.removeView(original)
        source.addView(original,FrameLayout.LayoutParams(-1,-1))
        container.addView(source,index,FrameLayout.LayoutParams(-1,-1))
        container.addView(compose, FrameLayout.LayoutParams(-1, -1))
        compose.setContent {
            val backdrop = remember { object : Backdrop {
                override val isCoordinatesDependent = true
                override fun DrawScope.drawBackdrop(density: Density, coordinates: LayoutCoordinates?, layerBlock: (GraphicsLayerScope.() -> Unit)?) {
                    frame // Invalidate only when the existing View tree has new content.
                    val coordinates = coordinates ?: return
                    val origin = IntArray(2); source.getLocationInWindow(origin)
                    val at = coordinates.positionInWindow()
                    drawIntoCanvas { canvas ->
                        val c = canvas.nativeCanvas
                        c.save(); c.translate(origin[0] - at.x, origin[1] - at.y)
                        try { if(source.layer.hasDisplayList()) c.drawRenderNode(source.layer) }
                        finally { c.restore() }
                    }
                }
            } }
            val active = state.enabled && running && Build.VERSION.SDK_INT >= 33
            Box(Modifier.fillMaxSize()) {
                if (active) Glass(backdrop, state)
            }
        }
    }
    fun configure(
        cssX: Float, cssY: Float, cssWidth: Float, cssHeight: Float, viewport: Float,
        selected: Int, position: Float, pressed: Boolean, dark: Boolean, ink: Int,
        reduced: Boolean, sequence: Long
    ): Boolean {
        if (disposed || sequence < revision || !supported(activity)) return false
        revision = sequence
        val ratio = web.width / viewport
        val origin = IntArray(2); val host = IntArray(2)
        web.getLocationInWindow(origin); compose.getLocationInWindow(host)
        state = State(true, origin[0] - host[0] + cssX * ratio,
            origin[1] - host[1] + cssY * ratio, cssWidth * ratio, cssHeight * ratio,
            ratio, selected, position, pressed, dark, ink, reduced)
        source.invalidate()
        return true
    }
    fun hide(sequence: Long) { if (sequence >= revision) { revision = sequence; state = state.copy(enabled = false); source.invalidate() } }
    fun pause() { running = false; source.invalidate() }
    fun resume() { running = true; source.invalidate() }
    fun active(): Boolean = state.enabled && running && !disposed
    fun dispose() {
        if (disposed) return
        disposed = true; state = state.copy(enabled = false)
        compose.disposeComposition(); container.removeView(compose)
        source.removeView(original); container.removeView(source)
        container.addView(original,FrameLayout.LayoutParams(-1,-1))
        source.layer.discardDisplayList()
    }
    @Composable private fun Glass(backdrop: Backdrop, s: State) {
        val density = LocalDensity.current
        val shape = RoundedCornerShape(50)
        val position by animateFloatAsState(s.position, tween(if (s.pressed || s.reduced) 0 else 230), label = "controlled-tab-position")
        val press by animateFloatAsState(if (s.pressed) 1f else 0f, tween(if(s.reduced) 0 else 140), label = "glass-press")
        val width = with(density) { s.width.toDp() }; val height = with(density) { s.height.toDp() }
        val inset = 8f * s.ratio
        val gap = 4f * s.ratio
        val tabWidth = (s.width - 2 * inset - 3 * gap) / 4f
        val tabHeight = s.height - 2 * inset
        Box(Modifier.offset { IntOffset(s.x.roundToInt(), s.y.roundToInt()) }.size(width, height)
            .drawBackdrop(
                backdrop = backdrop, shape = { shape },
                effects = { if (!s.reduced) { blur(1.2f.dp.toPx()); lens(15f.dp.toPx(), 11f.dp.toPx()) } },
                highlight = { Highlight(width = 0.65f.dp, alpha = if(s.dark) .55f else .9f) },
                shadow = { Shadow(radius = 8f.dp, alpha = .12f) },
                onDrawSurface = { drawRect(if(s.dark) Color(0xFF1C2936).copy(alpha=.12f) else Color.White.copy(alpha=.10f)) }
            )) {
            Box(Modifier.offset { IntOffset((inset + position * (tabWidth + gap)).roundToInt(), inset.roundToInt()) }
                .size(with(density) { tabWidth.toDp() }, with(density) { tabHeight.toDp() })
                .drawBackdrop(backdrop = backdrop, shape = { shape },
                    effects = { if(!s.reduced) lens((7f + 3f * press).dp.toPx(), (8f + 6f * press).dp.toPx()) },
                    highlight = { Highlight(width = .8f.dp, alpha = .8f) }, shadow = { null },
                    onDrawSurface = { drawRect(if(s.dark) Color.White.copy(alpha=.09f) else Color.White.copy(alpha=.16f)) }
                ))
            Canvas(Modifier.fillMaxSize()) {
                navDraws.incrementAndGet()
                drawIntoCanvas { canvas ->
                    val c = canvas.nativeCanvas
                    val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
                        color = s.ink; strokeWidth = 1.8f * s.ratio
                        strokeCap = Paint.Cap.ROUND; strokeJoin = Paint.Join.ROUND
                    }
                    val selectedLabel = if(s.pressed) s.position.roundToInt().coerceIn(0,3) else s.selected
                    val text = Paint(Paint.ANTI_ALIAS_FLAG).apply {
                        color = s.ink; textSize = 12f * s.ratio; typeface = android.graphics.Typeface.create("sans-serif", android.graphics.Typeface.BOLD)
                    }
                    for (i in 0..3) {
                        val label = labels[i]
                        val labelWidth = if(i==selectedLabel) text.measureText(label) + 5f*s.ratio else 0f
                        val iconSize = 23f*s.ratio
                        val x = inset + i*(tabWidth+gap) + (tabWidth-iconSize-labelWidth)/2
                        val y = (s.height-iconSize)/2
                        c.save();c.translate(x,y);c.scale(iconSize/24f,iconSize/24f)
                        paint.style=Paint.Style.STROKE;paint.strokeWidth=1.8f
                        c.drawPath(paths[i],paint);c.restore()
                        if(i==selectedLabel) c.drawText(label,x+iconSize+5*s.ratio,(s.height-text.fontMetrics.ascent-text.fontMetrics.descent)/2,text)
                    }
                }
            }
        }
    }
    companion object {
        private val labels = arrayOf("首页", "课表", "校园", "我的")
        private val paths = arrayOf(
            "M3 10 12 3 21 10V21H15V14H9V21H3Z",
            "M5 5H19Q21 5 21 7V20Q21 22 19 22H5Q3 22 3 20V7Q3 5 5 5ZM16 2V7M8 2V7M3 11H21M7 15H8M11.5 15H12.5M16 15H17M7 18.5H8M11.5 18.5H12.5M16 18.5H17",
            "M3 3H10V10H3ZM14 3H21V10H14ZM3 14H10V21H3ZM14 14H21V21H14Z",
            "M16 6A4 4 0 1 1 8 6A4 4 0 1 1 16 6M3 22V20A9 9 0 0 1 21 20V22"
        ).map { PathParser.createPathFromPathData(it)!! }
        @JvmStatic fun supported(activity: Activity): Boolean = Build.VERSION.SDK_INT >= 33 && activity.window.decorView.isHardwareAccelerated
        @JvmStatic fun install(activity: Activity, web: WebView): BackdropNavHost {
            val container = activity.findViewById<FrameLayout>(android.R.id.content)
            var original: View = web
            while (original.parent !== container) original = original.parent as? View ?: error("Existing WebView root not found")
            return BackdropNavHost(activity,web,container,original)
        }
    }
}
