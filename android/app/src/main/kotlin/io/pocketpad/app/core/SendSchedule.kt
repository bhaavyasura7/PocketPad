package io.pocketpad.app.core

/**
 * The sender's scheduling rules, pure and driven by an injectable clock so
 * the exact behaviour is unit tested. Microseconds everywhere.
 *
 * Rules (docs/protocol.md "Sending rules"):
 *  - a change is sent at once, at most at the negotiated rate;
 *  - while anything is held, the state is resent with the snapshot flag at
 *    least every 50 ms;
 *  - while nothing is held and nothing changes, no state is sent at all;
 *  - a heartbeat goes out every heartbeat interval.
 */
class SendSchedule(
    maxRateHz: Int,
    val heartbeatMs: Int = 500,
    val snapshotMs: Int = 50,
) {
    companion object {
        const val STATE = 1
        const val SNAPSHOT = 2
        const val HEARTBEAT = 4
    }

    private val minIntervalUs: Long = if (maxRateHz > 0) 1_000_000L / maxRateHz else 0L

    // Far enough in the past that the first poll is due, without overflow.
    private var lastStateUs = Long.MIN_VALUE / 4
    private var lastHeartbeatUs = Long.MIN_VALUE / 4

    /** What to send for the given moment. Returns a combination of the flags. */
    fun poll(nowUs: Long, changed: Boolean, held: Boolean): Int {
        var action = 0
        if (changed && nowUs - lastStateUs >= minIntervalUs) {
            action = action or STATE
        } else if (held && nowUs - lastStateUs >= snapshotMs * 1000L) {
            action = action or STATE or SNAPSHOT
        }
        if (heartbeatMs > 0 && nowUs - lastHeartbeatUs >= heartbeatMs * 1000L) {
            action = action or HEARTBEAT
        }
        return action
    }

    fun sentState(nowUs: Long) {
        lastStateUs = nowUs
    }

    fun sentHeartbeat(nowUs: Long) {
        lastHeartbeatUs = nowUs
    }

    /**
     * Microseconds until the loop should look again; the loop waits exactly
     * this long. [pendingChange] is true when a change was observed but the
     * rate limit delayed it (the loop must wake at the rate boundary).
     * Otherwise only the snapshot and heartbeat deadlines matter (a new
     * change wakes the loop by signal, so it never needs polling).
     */
    fun nextDueUs(nowUs: Long, pendingChange: Boolean, held: Boolean): Long {
        var due = Long.MAX_VALUE
        if (pendingChange) due = minOf(due, lastStateUs + minIntervalUs)
        if (heartbeatMs > 0) due = minOf(due, lastHeartbeatUs + heartbeatMs * 1000L)
        if (held) due = minOf(due, lastStateUs + snapshotMs * 1000L)
        return (due - nowUs).coerceAtLeast(0)
    }
}
