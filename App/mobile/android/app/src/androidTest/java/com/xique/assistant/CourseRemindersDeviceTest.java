package com.xique.assistant;

import android.app.Notification;
import android.app.NotificationManager;
import android.content.Context;
import android.os.PowerManager;
import android.os.SystemClock;

import androidx.test.platform.app.InstrumentationRegistry;

import org.json.JSONObject;
import org.junit.Test;

import java.time.DayOfWeek;
import java.time.ZonedDateTime;
import java.time.format.DateTimeFormatter;
import java.time.temporal.ChronoUnit;
import java.time.temporal.TemporalAdjusters;
import java.util.Arrays;
import java.util.UUID;
import java.util.concurrent.Callable;
import java.util.concurrent.TimeUnit;

import static org.junit.Assert.*;
import static org.junit.Assume.assumeTrue;

public class CourseRemindersDeviceTest {
    private static final DateTimeFormatter CLOCK = DateTimeFormatter.ofPattern("HH:mm");
    private final Context context = InstrumentationRegistry.getInstrumentation().getTargetContext();
    private final NotificationManager manager = context.getSystemService(NotificationManager.class);

    @Test public void testNotificationUsesCourseChannelWithoutChangingSchedule() throws Exception {
        long next = CourseReminders.state(context).getLong("next_at");
        try {
            assertTrue(CourseReminders.sendTestNotification(context));
            assertTrue(notificationVisible("上课提醒测试"));
            assertEquals(next, CourseReminders.state(context).getLong("next_at"));
        } finally {
            manager.cancel("course-reminder-test", 8741);
        }
    }

    @Test public void clockDeliversEditedCourseWhileTheScreenIsOff() throws Exception {
        assertTrue("Grant Alarms & reminders on the test emulator", CourseReminders.exact(context));
        var service = LocalCore.service(context);
        JSONObject bootstrap = service.bootstrap();
        assertEquals("Use an empty isolated emulator", 0, bootstrap.getJSONArray("courses").length());
        String workspace = bootstrap.getString("workspace_id");
        String syncId = UUID.randomUUID().toString();
        ZonedDateTime now = ZonedDateTime.now(CourseReminderPlanner.ZONE);
        ZonedDateTime first = now.plusMinutes(15).truncatedTo(ChronoUnit.MINUTES);
        JSONObject saved = service.saveCourse(workspace, bootstrap.getLong("revision"), 0, syncId,
                course("旧合成课程", first));
        long id = saved.getLong("course_id");
        try {
            configure(true, 5);
            long oldAlarm = CourseReminders.state(context).getLong("next_at");
            assertEquals(first.minusMinutes(5).toInstant().toEpochMilli(), oldAlarm);

            ZonedDateTime actual = now.plusMinutes(3).truncatedTo(ChronoUnit.MINUTES);
            service.saveCourse(workspace, saved.getLong("revision"), id, syncId, course("新合成课程", actual));
            sleepScreen();
            assertFalse(context.getSystemService(PowerManager.class).isInteractive());
            long before = System.currentTimeMillis();
            onAlarmThread(() -> { CourseReminders.schedule(context); return null; });
            long newAlarm = CourseReminders.state(context).getLong("next_at");
            assertTrue("Course inside the lead window is scheduled promptly", newAlarm > before && newAlarm < before + 5_000);
            haltUntilNotified("新合成课程");
            assertFalse(notificationVisible("旧合成课程"));
            onAlarmThread(() -> { CourseReminders.schedule(context); return null; });
            assertEquals("Refreshing must not repeat the same course", 0, CourseReminders.state(context).getLong("next_at"));
        } finally {
            configure(false, 5);
            service.deleteCourse(workspace, service.bootstrap().getLong("revision"), id);
            manager.cancelAll();
        }
    }

    @Test public void prepareScheduledCourseForReboot() throws Exception {
        assumeTrue("Only seed persistent data for an explicit reboot test",
                "true".equals(InstrumentationRegistry.getArguments().getString("verify_reboot")));
        var service = LocalCore.service(context);
        JSONObject bootstrap = service.bootstrap();
        assertEquals("Use an empty isolated emulator", 0, bootstrap.getJSONArray("courses").length());
        ZonedDateTime start = ZonedDateTime.now(CourseReminderPlanner.ZONE).plusMinutes(2).truncatedTo(ChronoUnit.MINUTES);
        service.saveCourse(bootstrap.getString("workspace_id"), bootstrap.getLong("revision"), 0,
                UUID.randomUUID().toString(), course("重启合成课程", start));
        configure(true, 0);
        assertEquals(start.toInstant().toEpochMilli(), CourseReminders.state(context).getLong("next_at"));
    }

    private static JSONObject course(String name, ZonedDateTime start) throws Exception {
        return new JSONObject().put("name", name).put("teacher", "").put("location", "测试教室")
                .put("color", "#86C0AD").put("day_of_week", start.getDayOfWeek().getValue())
                .put("start_time", start.format(CLOCK))
                .put("end_time", start.plusMinutes(20).format(CLOCK)).put("week_pattern", "1周")
                .put("semester_start_date", start.toLocalDate().with(TemporalAdjusters.previousOrSame(DayOfWeek.MONDAY)).toString())
                .put("semester", start.getYear() + "S2");
    }

    private void configure(boolean enabled, int minutes) throws Exception {
        onAlarmThread(() -> {
            assertTrue(CourseReminders.prefs(context).edit().putBoolean("enabled", enabled).putInt("minutes", minutes).commit());
            CourseReminders.schedule(context);
            return null;
        });
    }

    private static <T> T onAlarmThread(Callable<T> action) throws Exception {
        return CourseReminders.IO.submit(action).get(10, TimeUnit.SECONDS);
    }

    private boolean notificationVisible(String title) {
        return Arrays.stream(manager.getActiveNotifications()).anyMatch(item ->
                CourseReminders.CHANNEL.equals(item.getNotification().getChannelId())
                        && title.contentEquals(item.getNotification().extras.getCharSequence(Notification.EXTRA_TITLE, "")));
    }

    private void haltUntilNotified(String title) {
        long deadline = SystemClock.elapsedRealtime() + 25_000;
        while (SystemClock.elapsedRealtime() < deadline) {
            if (notificationVisible(title)) return;
            SystemClock.sleep(250);
        }
        fail("Notification was never posted to the system");
    }

    private void sleepScreen() throws Exception {
        InstrumentationRegistry.getInstrumentation().getUiAutomation()
                .executeShellCommand("input keyevent KEYCODE_SLEEP").close();
        PowerManager power = context.getSystemService(PowerManager.class);
        for (int i = 0; i < 25 && power.isInteractive(); i++) SystemClock.sleep(100);
    }
}
