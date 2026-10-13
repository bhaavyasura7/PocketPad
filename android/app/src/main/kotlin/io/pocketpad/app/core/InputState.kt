package io.pocketpad.app.core

import io.pocketpad.protocol.ControllerState

/**
 * The latest controller state, written by the UI thread and read by the
 * transport's sender thread. Both sides only copy a handful of fields inside
 * the lock, so neither ever blocks meaningfully. [onChanged] runs after the
 * write, outside the lock, to wake the sender.
 */
class InputState {
    private val lock = Any()
    private val current = ControllerState()

    @Volatile
    var onChanged: (() -> Unit)? = null

    /** Rebuilds the state from scratch (the touch view derives all inputs). */
    fun set(build: (ControllerState) -> Unit) {
        synchronized(lock) { build(current) }
        onChanged?.invoke()
    }

    /** Copies [state] into the latest state (the touch view's derived state). */
    fun write(state: ControllerState) {
        synchronized(lock) { current.copyFrom(state) }
        onChanged?.invoke()
    }

    fun clear() = set { it.clear() }

    /** Copies the latest state into [out]. */
    fun snapshot(out: ControllerState) {
        synchronized(lock) { out.copyFrom(current) }
    }
}

/** Layout-relevant, user-tunable view settings. */
data class UiSettings(
    val deadZone: Float = 0.10f,
    val triggerAnalog: Boolean = true,
    val haptic: Boolean = true,
    val leftHanded: Boolean = false,
    val layoutScale: Float = 1.0f,
    val stickDoubleTap: Boolean = false,
    val gamingLights: Boolean = true,
    /** 0 = flowing rainbow, 1 = solid colour. */
    val lightsStyle: Int = 0,
    val lightsSpeed: Float = 1.0f,
    val lightsBrightness: Float = 1.0f,
    val lightsColor: Int = 0xFF22D3EE.toInt(),
)
