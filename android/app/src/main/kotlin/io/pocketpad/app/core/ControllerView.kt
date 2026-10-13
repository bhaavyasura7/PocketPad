package io.pocketpad.app.core

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Path
import android.graphics.RectF
import android.util.AttributeSet
import android.view.HapticFeedbackConstants
import android.view.MotionEvent
import android.view.View
import io.pocketpad.protocol.Buttons
import io.pocketpad.protocol.ControllerState
import io.pocketpad.protocol.DPad

/**
 * The multi-touch controller surface. A custom View (not Compose gestures)
 * so pointer indices give reliable simultaneous touches at no per-frame cost
 * when idle. See docs/DEVELOPMENT.md, "Touch handling".
 */
class ControllerView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null,
) : View(context, attrs) {

    /** Where derived state is published. */
    var input: InputState? = null

    /** Live settings (read on every event so changes apply immediately). */
    var settingsProvider: () -> UiSettings = { UiSettings() }

    private val layout = Layout()
    private val scratch = ControllerState()

    // The state the last frame drew; also the previous button mask for haptics.
    private val drawn = ControllerState()

    private class Pointer(var control: Int, var x: Float, var y: Float, val stickClick: Boolean)

    private val pointers = HashMap<Int, Pointer>()

    private var lastLeftTapUpMs = 0L
    private var lastRightTapUpMs = 0L

    private var labelSize = 24f

    // Reused drawing objects: no allocation while drawing.
    private val fill = Paint(Paint.ANTI_ALIAS_FLAG)
    private val stroke = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE }
    private val text = Paint(Paint.ANTI_ALIAS_FLAG).apply { textAlign = Paint.Align.CENTER }
    private val tmpRect = RectF()
    private val tmpPath = Path()

    private val activeFill = 0xFF7C5CFF.toInt()
    private val idleFill = 0x14FFFFFF
    private val idleStroke = 0x33FFFFFF
    private val activeStroke = 0xFF9E86FF.toInt()
    private val labelColor = 0xCCFFFFFF.toInt()
    private val dimLabel = 0x66FFFFFF

    private val A_COLOR = 0xFF66BB6A.toInt()
    private val B_COLOR = 0xFFEF5350.toInt()
    private val X_COLOR = 0xFF42A5F5.toInt()
    private val Y_COLOR = 0xFFFFCA28.toInt()

    /** Applies new settings; recomputes the layout when geometry changed. */
    fun applySettings(s: UiSettings) {
        val old = settingsProvider()
        settingsProvider = { s }
        if (old.layoutScale != s.layoutScale || old.leftHanded != s.leftHanded) {
            layout.compute(width.toFloat(), height.toFloat(), s.layoutScale, s.leftHanded)
        }
        invalidate()
    }

    override fun onSizeChanged(w: Int, h: Int, oldw: Int, oldh: Int) {
        super.onSizeChanged(w, h, oldw, oldh)
        val s = settingsProvider()
        layout.compute(w.toFloat(), h.toFloat(), s.layoutScale, s.leftHanded)
        labelSize = h * 0.038f
        stroke.strokeWidth = h * 0.006f
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (!hasFocus) releaseAll()
    }

    override fun onDetachedFromWindow() {
        releaseAll()
        super.onDetachedFromWindow()
    }

    /** Releases everything: used on focus loss, detach and when stopping. */
    fun releaseAll() {
        if (pointers.isEmpty() && drawn.isNeutral) return
        pointers.clear()
        scratch.clear()
        publish(scratch, haptic = false, s = settingsProvider())
    }

    private fun zoneAt(x: Float, y: Float): Int = when {
        layout.lt.let { x >= it.left && x <= it.right && y >= it.top && y <= it.bottom } -> Control.LT
        layout.rt.let { x >= it.left && x <= it.right && y >= it.top && y <= it.bottom } -> Control.RT
        layout.lb.let { x >= it.left && x <= it.right && y >= it.top && y <= it.bottom } -> Control.BUTTON
        layout.rb.let { x >= it.left && x <= it.right && y >= it.top && y <= it.bottom } -> Control.BUTTON
        layout.lsClick.hit(x, y) -> Control.LS_CLICK
        layout.rsClick.hit(x, y) -> Control.RS_CLICK
        layout.dpad.hit(x, y) -> Control.DPAD
        layout.leftStick.hit(x, y) -> Control.LEFT_STICK
        layout.rightStick.hit(x, y) -> Control.RIGHT_STICK
        layout.a.hit(x, y) || layout.b.hit(x, y) || layout.x.hit(x, y) || layout.y.hit(x, y) ||
            layout.back.hit(x, y) || layout.start.hit(x, y) || layout.guide.hit(x, y) -> Control.BUTTON
        else -> Control.NONE
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        val s = settingsProvider()
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_POINTER_DOWN -> {
                val i = event.actionIndex
                capture(event.getPointerId(i), event.getX(i), event.getY(i), event.eventTime, s)
            }
            MotionEvent.ACTION_MOVE -> {
                for (i in 0 until event.pointerCount) {
                    val p = pointers[event.getPointerId(i)] ?: continue
                    p.x = event.getX(i)
                    p.y = event.getY(i)
                }
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_POINTER_UP -> {
                val i = event.actionIndex
                release(event.getPointerId(i), event.eventTime)
            }
            MotionEvent.ACTION_CANCEL -> {
                pointers.clear()
            }
            else -> return super.onTouchEvent(event)
        }
        buildAndPublish(s, haptic = s.haptic)
        return true
    }

    private fun capture(id: Int, x: Float, y: Float, nowMs: Long, s: UiSettings) {
        if (pointers.containsKey(id)) return
        var zone = zoneAt(x, y)
        // One pointer per exclusive control.
        if (zone != Control.BUTTON && zone != Control.NONE && pointers.values.any { it.control == zone }) {
            zone = Control.NONE
        }
        var stickClick = false
        if (s.stickDoubleTap) {
            when (zone) {
                Control.LEFT_STICK -> if (nowMs - lastLeftTapUpMs in 1..320) stickClick = true
                Control.RIGHT_STICK -> if (nowMs - lastRightTapUpMs in 1..320) stickClick = true
            }
        }
        pointers[id] = Pointer(zone, x, y, stickClick)
    }

    private fun release(id: Int, nowMs: Long) {
        val p = pointers.remove(id) ?: return
        when (p.control) {
            Control.LEFT_STICK -> lastLeftTapUpMs = nowMs
            Control.RIGHT_STICK -> lastRightTapUpMs = nowMs
        }
    }

    private fun buildAndPublish(s: UiSettings, haptic: Boolean) {
        buildState(scratch, s)
        publish(scratch, haptic, s)
    }

    private fun publish(state: ControllerState, haptic: Boolean, s: UiSettings) {
        val gained = state.buttons and drawn.buttons.inv()
        val changed = !state.sameOnWire(drawn)
        if (changed) {
            input?.write(state)
            drawn.copyFrom(state)
            invalidate()
        }
        if (haptic && gained != 0) {
            performHapticFeedback(HapticFeedbackConstants.KEYBOARD_TAP)
        }
    }

    /** Derives the whole controller state from the current pointers. */
    private fun buildState(into: ControllerState, s: UiSettings) {
        into.clear()
        for ((_, p) in pointers.entries.sortedBy { it.key }) {
            when (p.control) {
                Control.LEFT_STICK -> {
                    applyStick(into, left = true, p.x, p.y, s.deadZone)
                    if (p.stickClick) into.buttons = into.buttons or Buttons.LS
                }
                Control.RIGHT_STICK -> {
                    applyStick(into, left = false, p.x, p.y, s.deadZone)
                    if (p.stickClick) into.buttons = into.buttons or Buttons.RS
                }
                Control.LT -> into.lt = triggerValue(p.y, layout.lt, s.triggerAnalog)
                Control.RT -> into.rt = triggerValue(p.y, layout.rt, s.triggerAnalog)
                Control.DPAD -> into.dpad = Mapping.dpadSector(p.x - layout.dpad.cx, p.y - layout.dpad.cy, layout.dpad.r)
                Control.BUTTON -> into.buttons = into.buttons or buttonAt(p.x, p.y)
                Control.LS_CLICK -> into.buttons = into.buttons or Buttons.LS
                Control.RS_CLICK -> into.buttons = into.buttons or Buttons.RS
                Control.NONE -> {}
            }
        }
    }

    private fun applyStick(into: ControllerState, left: Boolean, px: Float, py: Float, dz: Float) {
        val c = if (left) layout.leftStick else layout.rightStick
        val v = Mapping.deadZone((px - c.cx) / c.r, (py - c.cy) / c.r, dz) ?: return
        if (left) {
            into.lx = v[0]; into.ly = -v[1]
        } else {
            into.rx = v[0]; into.ry = -v[1]
        }
    }

    private fun buttonAt(x: Float, y: Float): Int = when {
        layout.lb.let { x >= it.left && x <= it.right && y >= it.top && y <= it.bottom } -> Buttons.LB
        layout.rb.let { x >= it.left && x <= it.right && y >= it.top && y <= it.bottom } -> Buttons.RB
        layout.a.hit(x, y, 0.15f) -> Buttons.A
        layout.b.hit(x, y, 0.15f) -> Buttons.B
        layout.x.hit(x, y, 0.15f) -> Buttons.X
        layout.y.hit(x, y, 0.15f) -> Buttons.Y
        layout.back.hit(x, y, 0.15f) -> Buttons.BACK
        layout.start.hit(x, y, 0.15f) -> Buttons.START
        layout.guide.hit(x, y, 0.15f) -> Buttons.GUIDE
        else -> 0
    }

    private fun triggerValue(py: Float, b: Bounds, analog: Boolean): Float {
        val p = ((py - b.top) / b.height).coerceIn(0f, 1f)
        return Mapping.triggerValue(p, analog)
    }

    // --- drawing ---

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        drawTrigger(canvas, layout.lt, drawn.lt)
        drawTrigger(canvas, layout.rt, drawn.rt)
        drawTriggerSideLabel(canvas, layout.lt, "LT")
        drawTriggerSideLabel(canvas, layout.rt, "RT")
        drawPill(canvas, layout.lb, "LB", drawn.buttons and Buttons.LB != 0)
        drawPill(canvas, layout.rb, "RB", drawn.buttons and Buttons.RB != 0)

        drawCircleButton(canvas, layout.back, "SEL", drawn.buttons and Buttons.BACK != 0, labelColor)
        drawCircleButton(canvas, layout.start, "STA", drawn.buttons and Buttons.START != 0, labelColor)
        drawCircleButton(canvas, layout.guide, "PP", drawn.buttons and Buttons.GUIDE != 0, labelColor)

        drawCircleButton(canvas, layout.a, "A", drawn.buttons and Buttons.A != 0, A_COLOR)
        drawCircleButton(canvas, layout.b, "B", drawn.buttons and Buttons.B != 0, B_COLOR)
        drawCircleButton(canvas, layout.x, "X", drawn.buttons and Buttons.X != 0, X_COLOR)
        drawCircleButton(canvas, layout.y, "Y", drawn.buttons and Buttons.Y != 0, Y_COLOR)

        drawDpad(canvas)
        drawStick(canvas, layout.leftStick, drawn.lx, drawn.ly)
        drawStick(canvas, layout.rightStick, drawn.rx, drawn.ry)
        drawCircleButton(canvas, layout.lsClick, "L3", drawn.buttons and Buttons.LS != 0, labelColor)
        drawCircleButton(canvas, layout.rsClick, "R3", drawn.buttons and Buttons.RS != 0, labelColor)
    }

    private fun drawTrigger(canvas: Canvas, b: Bounds, value: Float) {
        tmpRect.set(b.left, b.top, b.right, b.bottom)
        val radius = b.width * 0.18f
        fill.color = idleFill
        canvas.drawRoundRect(tmpRect, radius, radius, fill)
        if (value > 0.001f) {
            fill.color = withAlpha(activeFill, 0.45f)
            val filled = b.bottom - b.height * value.coerceIn(0f, 1f)
            tmpRect.set(b.left, filled, b.right, b.bottom)
            canvas.drawRoundRect(tmpRect, radius, radius, fill)
        }
        stroke.color = if (value > 0.001f) activeStroke else idleStroke
        tmpRect.set(b.left, b.top, b.right, b.bottom)
        canvas.drawRoundRect(tmpRect, radius, radius, stroke)
    }

    private fun drawTriggerSideLabel(canvas: Canvas, b: Bounds, label: String) {
        text.color = dimLabel
        text.textSize = labelSize * 0.7f
        canvas.drawText(label, b.left + b.width / 2f, b.top + text.textSize * 1.3f, text)
    }

    private fun drawPill(canvas: Canvas, b: Bounds, label: String, active: Boolean) {
        tmpRect.set(b.left, b.top, b.right, b.bottom)
        val radius = b.height * 0.35f
        fill.color = if (active) withAlpha(activeFill, 0.55f) else idleFill
        canvas.drawRoundRect(tmpRect, radius, radius, fill)
        stroke.color = if (active) activeStroke else idleStroke
        canvas.drawRoundRect(tmpRect, radius, radius, stroke)
        text.color = if (active) 0xFFFFFFFF.toInt() else dimLabel
        text.textSize = labelSize * 0.72f
        canvas.drawText(label, b.left + b.width / 2f, b.top + b.height * 0.68f, text)
    }

    private fun drawCircleButton(canvas: Canvas, c: Circle, label: String, active: Boolean, tint: Int) {
        fill.color = if (active) withAlpha(tint, 0.5f) else idleFill
        canvas.drawCircle(c.cx, c.cy, c.r, fill)
        stroke.color = if (active) tint else idleStroke
        canvas.drawCircle(c.cx, c.cy, c.r, stroke)
        text.color = if (active) 0xFFFFFFFF.toInt() else labelColor
        text.textSize = if (c.r < labelSize * 1.2f) labelSize * 0.6f else labelSize * 0.9f
        canvas.drawText(label, c.cx, c.cy + text.textSize * 0.36f, text)
    }

    private fun drawStick(canvas: Canvas, c: Circle, lx: Float, ly: Float) {
        fill.color = idleFill
        canvas.drawCircle(c.cx, c.cy, c.r, fill)
        stroke.color = idleStroke
        canvas.drawCircle(c.cx, c.cy, c.r, stroke)
        stroke.color = 0x22FFFFFF
        canvas.drawCircle(c.cx, c.cy, c.r * 0.25f, stroke)

        val kx = c.cx + lx * c.r * 0.62f
        val ky = c.cy - ly * c.r * 0.62f
        val kr = c.r * 0.42f
        val active = lx != 0f || ly != 0f
        fill.color = if (active) withAlpha(activeFill, 0.55f) else 0x33FFFFFF
        canvas.drawCircle(kx, ky, kr, fill)
        stroke.color = if (active) activeStroke else idleStroke
        canvas.drawCircle(kx, ky, kr, stroke)
    }

    private fun drawDpad(canvas: Canvas) {
        val c = layout.dpad
        fill.color = idleFill
        canvas.drawCircle(c.cx, c.cy, c.r, fill)
        stroke.color = idleStroke
        canvas.drawCircle(c.cx, c.cy, c.r, stroke)

        val arm = c.r * 0.34f
        val bar = c.r * 0.30f
        val d = drawn.dpad
        // Four arrows as rounded bars from the centre.
        drawArrow(canvas, c.cx, c.cy - arm / 2f - arm, bar, arm, DPad.up(d))     // up
        drawArrow(canvas, c.cx, c.cy + arm / 2f + arm, bar, arm, DPad.down(d))   // down
        drawArrow(canvas, c.cx - arm / 2f - arm, c.cy, arm, bar, DPad.left(d))   // left
        drawArrow(canvas, c.cx + arm / 2f + arm, c.cy, arm, bar, DPad.right(d))  // right
        // Centre dot.
        fill.color = idleFill
        canvas.drawCircle(c.cx, c.cy, c.r * 0.16f, fill)
    }

    private fun drawArrow(canvas: Canvas, cx: Float, cy: Float, w: Float, h: Float, active: Boolean) {
        tmpPath.reset()
        tmpPath.addRoundRect(cx - w / 2f, cy - h / 2f, cx + w / 2f, cy + h / 2f, w * 0.25f, h * 0.25f, Path.Direction.CW)
        fill.color = if (active) withAlpha(activeFill, 0.7f) else 0x26FFFFFF
        canvas.drawPath(tmpPath, fill)
    }

    private fun withAlpha(color: Int, fraction: Float): Int {
        val a = ((color ushr 24) * fraction).toInt().coerceIn(0, 255)
        return (color and 0x00FFFFFF) or (a shl 24)
    }
}

/** Pointer control ids. */
object Control {
    const val NONE = 0
    const val LEFT_STICK = 1
    const val RIGHT_STICK = 2
    const val LT = 3
    const val RT = 4
    const val DPAD = 5
    const val BUTTON = 6
    const val LS_CLICK = 7
    const val RS_CLICK = 8
}
