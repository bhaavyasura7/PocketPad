package io.pocketpad.app.core

import android.content.Context
import android.os.BatteryManager

/** Reads the phone battery for the heartbeat and the controller overlay. */
class BatteryReader(private val context: Context) {

    data class Reading(val percent: Int, val charging: Boolean)

    fun read(): Reading {
        val bm = context.getSystemService(Context.BATTERY_SERVICE) as? BatteryManager
            ?: return Reading(255, false)
        val capacity = try {
            bm.getIntProperty(BatteryManager.BATTERY_PROPERTY_CAPACITY)
        } catch (_: Exception) {
            -1
        }
        val charging = try {
            bm.isCharging
        } catch (_: Exception) {
            false
        }
        return Reading(if (capacity in 0..100) capacity else 255, charging)
    }
}
