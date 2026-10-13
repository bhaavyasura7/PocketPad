package io.pocketpad.app

import io.pocketpad.app.core.SendSchedule
import io.pocketpad.app.core.SendSchedule.Companion.HEARTBEAT
import io.pocketpad.app.core.SendSchedule.Companion.SNAPSHOT
import io.pocketpad.app.core.SendSchedule.Companion.STATE
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotEquals
import kotlin.test.assertTrue

class SendScheduleTest {

    private fun fresh() = SendSchedule(maxRateHz = 250, heartbeatMs = 500, snapshotMs = 50)

    @Test
    fun firstPollIsDue() {
        val s = fresh()
        val a = s.poll(1_000_000, changed = true, held = true)
        assertTrue(a and STATE != 0)
        assertTrue(a and HEARTBEAT != 0)
        assertTrue(a and SNAPSHOT == 0) // a change is not a snapshot
    }

    @Test
    fun rateLimitCoalescesChanges() {
        val s = fresh() // 250 Hz -> 4000 us
        s.sentState(1_000_000)
        s.sentHeartbeat(1_000_000)
        assertEquals(0, s.poll(1_002_000, changed = true, held = false) and STATE)
        assertNotEquals(0, s.poll(1_004_000, changed = true, held = false) and STATE)
    }

    @Test
    fun heldStateResendsWithSnapshot() {
        val s = fresh()
        s.sentState(1_000_000)
        s.sentHeartbeat(1_000_000)
        val early = s.poll(1_030_000, changed = false, held = true)
        assertEquals(0, early and SNAPSHOT)
        val due = s.poll(1_050_000, changed = false, held = true)
        assertNotEquals(0, due and STATE)
        assertNotEquals(0, due and SNAPSHOT)
    }

    @Test
    fun idleUnchangedSendsNothing() {
        val s = fresh()
        s.sentState(1_000_000)
        s.sentHeartbeat(1_000_000)
        assertEquals(0, s.poll(1_100_000, changed = false, held = false))
    }

    @Test
    fun heartbeatCadence() {
        val s = fresh()
        s.sentHeartbeat(0)
        assertEquals(0, s.poll(400_000, changed = false, held = false) and HEARTBEAT)
        assertNotEquals(0, s.poll(500_000, changed = false, held = false) and HEARTBEAT)
    }

    @Test
    fun heartbeatDisabledForBluetooth() {
        val s = SendSchedule(maxRateHz = 125, heartbeatMs = 0, snapshotMs = 100)
        s.sentHeartbeat(0)
        assertEquals(0, s.poll(10_000_000, changed = false, held = false) and HEARTBEAT)
    }

    @Test
    fun nextDueTracksTheNearestDeadline() {
        val s = fresh()
        s.sentState(1_000_000)
        s.sentHeartbeat(1_000_000)
        // No pending change, nothing held: only the heartbeat is due.
        assertEquals(500_000, s.nextDueUs(1_000_000, pendingChange = false, held = false))
        // A change delayed by the rate limit is due at the rate boundary.
        assertEquals(4_000, s.nextDueUs(1_000_000, pendingChange = true, held = false))
        // Held: the 50 ms snapshot deadline is nearer than the heartbeat.
        assertEquals(50_000, s.nextDueUs(1_000_000, pendingChange = false, held = true))
        assertEquals(40_000, s.nextDueUs(1_010_000, pendingChange = false, held = true))
        // Pending and overdue: wake immediately.
        assertEquals(0, s.nextDueUs(1_010_000, pendingChange = true, held = true))
    }
}
