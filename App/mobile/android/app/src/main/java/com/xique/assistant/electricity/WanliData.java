package com.xique.assistant.electricity;

import java.math.BigDecimal;
import java.time.LocalDate;
import java.time.YearMonth;
import java.util.*;

/** Value/shape checks shared by native query code. Contains no credentials or IO. */
final class WanliData {
    static final class Invalid extends Exception {
        Invalid() { super("UPSTREAM_DATA_INVALID"); }
    }
    static final class Meter {
        private final String gatherCode;
        private final Map<String,Object> display;
        private Meter(String code, Map<String,Object> view) {
            gatherCode=code; display=Collections.unmodifiableMap(view);
        }
        String codeForOwnedRequest() { return gatherCode; }
        Map<String,Object> display() { return display; }
    }

    static List<Meter> meters(Object value) throws Invalid {
        if (!(value instanceof List) || ((List<?>)value).size()>100) throw new Invalid();
        List<Meter> result=new ArrayList<>(); Set<String> unique=new HashSet<>();
        for (Object item:(List<?>)value) {
            Map<?,?> row=object(item);
            // Only the electricity type actually verified against the user's Mini Program.
            if (!"1".equals(row.get("meterType"))) continue;
            String code=text(row.get("gatherCode"),128,false);
            if (!unique.add(code)) throw new Invalid();
            Map<String,Object> view=new LinkedHashMap<>();
            view.put("remaining_kwh",decimal(row.get("remainingPower"),false));
            view.put("reading_time",text(row.get("readingTime"),64,true));
            view.put("cumulative_kwh",decimal(row.get("cumulativePower"),true));
            result.add(new Meter(code,view));
        }
        return Collections.unmodifiableList(result);
    }

    static List<Map<String,Object>> days(Object value,String month) throws Invalid {
        YearMonth selected=month(month); Map<?,?> data=object(value);
        Object raw=data.get("list");
        if (!(raw instanceof List) || ((List<?>)raw).size()>31) throw new Invalid();
        SortedMap<LocalDate,Map<String,Object>> dates=new TreeMap<>();
        for(Object item:(List<?>)raw) {
            Map<?,?> row=object(item); LocalDate date;
            try { date=LocalDate.of(integer(row.get("year")),integer(row.get("month")),integer(row.get("day"))); }
            catch(RuntimeException error) { throw new Invalid(); }
            if(!YearMonth.from(date).equals(selected) || dates.containsKey(date)) throw new Invalid();
            Object rawAmount=row.get("electricityCount");
            if(!(rawAmount instanceof String)) throw new Invalid();
            String amount=decimal(rawAmount,false);
            String start=decimal(row.get("startMeterNumber"),true);
            String end=decimal(row.get("endMeterNumber"),true);
            boolean reading=start!=null && end!=null;
            Map<String,Object> day=new LinkedHashMap<>();
            day.put("date",date.toString()); day.put("state",reading?"READING":"NO_READING");
            day.put("kwh",reading?amount:null); day.put("source_kwh",amount);
            day.put("start_reading",start); day.put("end_reading",end);
            day.put("begin_time",text(row.get("beginTime"),64,true));
            day.put("end_time",text(row.get("endTime"),64,true));
            dates.put(date,Collections.unmodifiableMap(day));
        }
        return Collections.unmodifiableList(new ArrayList<>(dates.values()));
    }

    static YearMonth month(String value) throws Invalid {
        if(value==null || !value.matches("20[0-9]{2}-(0[1-9]|1[0-2])")) throw new Invalid();
        try { return YearMonth.parse(value); } catch(RuntimeException e) { throw new Invalid(); }
    }
    private static Map<?,?> object(Object v) throws Invalid {
        if(!(v instanceof Map))throw new Invalid();return (Map<?,?>)v;
    }
    private static int integer(Object v) throws Invalid {
        if(!(v instanceof Number))throw new Invalid();
        Number n=(Number)v; int i=n.intValue();if(n.doubleValue()!=i)throw new Invalid();return i;
    }
    private static String text(Object v,int limit,boolean nullable) throws Invalid {
        if(v==null || "".equals(v)) { if(nullable)return null;throw new Invalid(); }
        if(!(v instanceof String) || ((String)v).length()>limit)throw new Invalid();
        String s=(String)v;for(int i=0;i<s.length();i++)if(s.charAt(i)<32 || s.charAt(i)==127)throw new Invalid();
        return s;
    }
    private static String decimal(Object v,boolean nullable) throws Invalid {
        if(v==null) { if(nullable)return null;throw new Invalid(); }
        if(!(v instanceof String) && !(v instanceof Number))throw new Invalid();
        String s=v.toString();
        if(s.length()>64 || (v instanceof String && !s.matches("-?[0-9]+(?:\\.[0-9]+)?")))throw new Invalid();
        try {
            BigDecimal n=new BigDecimal(s);
            if(n.precision()>40 || Math.abs(n.scale())>20)throw new Invalid();
            return v instanceof String?s:n.toPlainString();
        } catch(NumberFormatException e) { throw new Invalid(); }
    }
}
