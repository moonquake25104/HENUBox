package com.xique.assistant;

import java.time.DateTimeException;
import java.time.DayOfWeek;
import java.time.LocalDate;
import java.time.LocalTime;
import java.time.temporal.ChronoUnit;
import java.time.temporal.TemporalAdjusters;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import org.json.JSONArray;
import org.json.JSONObject;

final class WidgetTimetableData {
    private WidgetTimetableData() {}

    static List<Lesson> lessons(JSONArray rows, LocalDate date, LocalTime cutoff) {
        List<Lesson> lessons = new ArrayList<>();
        for (int index = 0; index < rows.length(); index++) {
            JSONObject row = rows.optJSONObject(index);
            if (row == null || row.optInt("day_of_week") != date.getDayOfWeek().getValue()) continue;
            try {
                LocalDate anchor = LocalDate.parse(row.optString("semester_start_date"))
                        .with(TemporalAdjusters.previousOrSame(DayOfWeek.MONDAY));
                if (date.isBefore(anchor)) continue;
                int week = 1 + (int) (ChronoUnit.DAYS.between(anchor, date) / 7);
                if (!CourseReminderPlanner.matches(row.optString("week_pattern"), week)) continue;
                LocalTime start = LocalTime.parse(row.optString("start_time")).withSecond(0).withNano(0);
                LocalTime end = LocalTime.parse(row.optString("end_time")).withSecond(0).withNano(0);
                if (!end.isAfter(cutoff) || !end.isAfter(start)) continue;
                String name = row.optString("name"), room = row.optString("location");
                if (name.isEmpty()) continue;
                Lesson lesson = new Lesson(name, room, start, end);
                if (lessons.stream().noneMatch(item -> item.sameAs(lesson))) lessons.add(lesson);
            } catch (DateTimeException | NumberFormatException ignored) {
                // Keep the remaining valid saved courses visible.
            }
        }
        lessons.sort(Comparator.comparing(item -> item.start));
        return lessons;
    }

    static final class Lesson {
        final String name, room;
        final LocalTime start, end;

        Lesson(String name, String room, LocalTime start, LocalTime end) {
            this.name = name;
            this.room = room;
            this.start = start;
            this.end = end;
        }

        boolean sameAs(Lesson other) {
            return name.equals(other.name) && room.equals(other.room) && start.equals(other.start) && end.equals(other.end);
        }
    }
}
