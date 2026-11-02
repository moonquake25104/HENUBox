package com.xique.assistant;

import org.junit.Test;
import static org.junit.Assert.*;
import java.time.*;
import java.util.*;

public class CourseReminderPlannerTest {
    private long at(String time){return ZonedDateTime.parse(time+"+08:00[Asia/Shanghai]").toInstant().toEpochMilli();}
    private CourseReminderPlanner.Course course(String pattern,String start,int day){return new CourseReminderPlanner.Course(1,"合成课程","","2026-08-31",start,pattern,day);}
    @Test public void exactLeadAndEighteenWeekLimit(){
        var plan=CourseReminderPlanner.future(List.of(course("1-18周全周","08:30",1)),at("2026-08-30T20:00"),10);
        assertEquals(18,plan.size());assertEquals(at("2026-08-31T08:20"),plan.get(0).at);assertEquals(at("2026-12-28T08:30"),plan.get(17).start);
        assertTrue(CourseReminderPlanner.future(List.of(course("1-18周全周","08:30",1)),at("2026-12-29T00:00"),10).isEmpty());
    }
    @Test public void oddEvenAndNonContinuousWeeks(){
        assertTrue(CourseReminderPlanner.matches("1-18周(单周)",3));assertFalse(CourseReminderPlanner.matches("1-18周(单周)",2));
        assertTrue(CourseReminderPlanner.matches("1-18周双周",18));assertFalse(CourseReminderPlanner.matches("1-18周双周",17));
        assertTrue(CourseReminderPlanner.matches("1周,3周,5-7周",6));assertFalse(CourseReminderPlanner.matches("1周,3周,5-7周",2));assertFalse(CourseReminderPlanner.matches("1-30周",19));
    }
    @Test public void crossMidnightAndPassedReminderNotReplayed(){
        var courses=List.of(course("1周","00:30",1));var plan=CourseReminderPlanner.future(courses,at("2026-08-30T22:00"),60);
        assertEquals(at("2026-08-30T23:30"),plan.get(0).at);assertTrue(CourseReminderPlanner.future(courses,at("2026-08-30T23:31"),60).isEmpty());
    }
    @Test public void invalidRowsAndUnknownPatternDoNotCreateAlarms(){
        assertTrue(CourseReminderPlanner.future(List.of(course("未知周次","08:30",1),course("1周","26:00",1),course("1周","08:30",0)),at("2026-08-30T20:00"),10).isEmpty());
    }
    @Test public void simultaneousCoursesAreNotDropped(){
        var a=course("1周","08:30",1);var b=new CourseReminderPlanner.Course(2,"合成课程二","","2026-08-31","08:30","1周",1);
        var plan=CourseReminderPlanner.future(List.of(b,a),at("2026-08-30T20:00"),0);assertEquals(2,plan.size());assertEquals(1,plan.get(0).course.id);assertNotEquals(plan.get(0).key,plan.get(1).key);
    }
    @Test(expected=IllegalArgumentException.class) public void excessiveLeadRejected(){CourseReminderPlanner.future(List.of(),0,121);}
    @Test public void exactLegacyDuplicateOnlyRemindsOnce(){
        var a=course("1周","08:30",1);var b=new CourseReminderPlanner.Course(22,a.name,a.location,a.anchor,a.start,a.pattern,a.weekday);
        assertEquals(1,CourseReminderPlanner.future(List.of(a,b),at("2026-08-30T20:00"),10).size());
        assertTrue(CourseReminderPlanner.matches("第1-18周全周",18));assertTrue(CourseReminderPlanner.matches("1周/3周",3));assertTrue(CourseReminderPlanner.matches("全",8));
    }
}
