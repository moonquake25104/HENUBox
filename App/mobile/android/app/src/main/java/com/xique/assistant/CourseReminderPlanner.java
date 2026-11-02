package com.xique.assistant;

import java.time.*;
import java.time.temporal.ChronoUnit;
import java.time.temporal.TemporalAdjusters;
import java.util.*;
import java.util.regex.*;

/** Pure calendar calculation, independent of alarms, permissions and Android. */
final class CourseReminderPlanner {
    static final ZoneId ZONE=ZoneId.of("Asia/Shanghai");
    static final class Course {
        final long id; final String name,location,anchor,start,pattern; final int weekday;
        Course(long id,String name,String location,String anchor,String start,String pattern,int weekday){this.id=id;this.name=name;this.location=location;this.anchor=anchor;this.start=start;this.pattern=pattern;this.weekday=weekday;}
    }
    static final class Event {
        final Course course; final long start,at; final String key;
        Event(Course c,long start,int minutes){course=c;this.start=start;at=start-minutes*60000L;key=c.id+":"+start;}
    }
    static List<Event> future(List<Course> courses,long now,int minutes){
        if(minutes<0||minutes>120)throw new IllegalArgumentException("INVALID_ARGUMENT");
        List<Event> events=new ArrayList<>();Set<List<Object>> seen=new HashSet<>();
        for(Course c:courses)try{
            if(c.weekday<1||c.weekday>7||c.name.trim().isEmpty())continue;
            LocalDate anchor=LocalDate.parse(c.anchor).with(TemporalAdjusters.previousOrSame(DayOfWeek.MONDAY));
            LocalTime time=LocalTime.parse(c.start);
            for(int w=1;w<=18;w++)if(matches(c.pattern,w)){
                long start=anchor.plusDays((w-1)*7L+c.weekday-1).atTime(time).atZone(ZONE).toInstant().toEpochMilli();
                Event event=new Event(c,start,minutes);if(event.at>now&&seen.add(Arrays.asList(c.name,c.location,start)))events.add(event);
            }
        }catch(DateTimeException ignored){/* A malformed course never invents an alarm. */}
        events.sort(Comparator.comparingLong((Event e)->e.at).thenComparingLong(e->e.course.id));return events;
    }
    static boolean matches(String pattern,int week){
        if(week<1||week>18)return false;
        String p=pattern==null?"":pattern.trim().replace('，',',').replace('、',',').replace('；',',').replace('－','-').replace('–','-').replace('—','-').replaceAll("\\s+","");
        if(p.isEmpty()||p.equals("全周")||p.equals("全")||p.equals("每周"))return true;
        for(String segment:p.split("[,;/]+")){
            String part=segment.replaceFirst("^第","");
            Matcher m=Pattern.compile("^(\\d+)(?:-(\\d+))?(?:周)?(?:(?:[（(])?(单周?|双周?|全周?)(?:[）)])?)?$").matcher(part);
            if(part.equals("单周")||part.equals("单")){if(week%2==1)return true;continue;}
            if(part.equals("双周")||part.equals("双")){if(week%2==0)return true;continue;}
            if(!m.matches())continue;
            int a=Integer.parseInt(m.group(1)),b=m.group(2)==null?a:Integer.parseInt(m.group(2));String parity=m.group(3);
            if(week>=a&&week<=b&&(parity==null||(!parity.startsWith("单")&&!parity.startsWith("双"))||(parity.startsWith("单")?week%2==1:week%2==0)))return true;
        }
        return false;
    }
}
