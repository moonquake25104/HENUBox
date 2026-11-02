package com.xique.assistant.electricity;

import org.junit.Test;
import java.util.*;
import static org.junit.Assert.*;

/** All synthetic, offline; never connects, reads credentials or touches the phone. */
public class WanliDataTest {
    private Map<String,Object> row(Object... fields) {
        Map<String,Object> r=new LinkedHashMap<>();for(int i=0;i<fields.length;i+=2)r.put((String)fields[i],fields[i+1]);return r;
    }
    private Map<String,Object> day(int date,Object start,Object end,String amount) {
        return row("year",2026,"month",10,"day",date,"startMeterNumber",start,"endMeterNumber",end,"electricityCount",amount);
    }
    @Test public void noReadingIsNotAConfirmedZero() throws Exception {
        List<Map<String,Object>> days=WanliData.days(row("list",Arrays.asList(day(2,null,null,"0"),day(1,"10","10","0.0000"))),"2026-10");
        assertEquals("2026-10-01",days.get(0).get("date"));assertEquals("READING",days.get(0).get("state"));
        assertEquals("0.0000",days.get(0).get("kwh"));assertEquals("NO_READING",days.get(1).get("state"));
        assertNull(days.get(1).get("kwh"));assertEquals("0",days.get(1).get("source_kwh"));
    }
    @Test public void decimalPrecisionIsPreservedAndNotRecomputed() throws Exception {
        Map<String,Object> value=WanliData.days(row("list",Collections.singletonList(day(1,"10.00000001","10.00000002","0.000000009"))),"2026-10").get(0);
        assertEquals("0.000000009",value.get("kwh"));assertEquals("10.00000001",value.get("start_reading"));
    }
    @Test public void wrongMonthAndDuplicateDatesAreRejected() throws Exception {
        rejects(()->WanliData.days(row("list",Arrays.asList(day(1,"1","2","1"),day(1,"1","2","1"))),"2026-10"));
        rejects(()->WanliData.days(row("list",Collections.singletonList(day(1,"1","2","1"))),"2026-09"));
        rejects(()->WanliData.days(row("list",Collections.singletonList(day(32,"1","2","1"))),"2026-10"));
    }
    @Test public void invalidAmountCannotTurnIntoZero() throws Exception {
        for(String amount:Arrays.asList("NaN","","1e100","Infinity","2kWh"))
            rejects(()->WanliData.days(row("list",Collections.singletonList(day(1,"1","2",amount))),"2026-10"));
    }
    @Test public void onlyVerifiedElectricityTypeAndNoIdentifiersInDisplay() throws Exception {
        Map<String,Object> electric=row("meterType","1","gatherCode","synthetic-owned-meter","remainingPower",12.25,"readingTime","2026-10-01 10:00:00","userId","synthetic-private");
        Map<String,Object> water=row("meterType","unverified","gatherCode","not-an-electric-meter","remainingPower",99);
        List<WanliData.Meter> meters=WanliData.meters(Arrays.asList(electric,water));assertEquals(1,meters.size());
        assertEquals("12.25",meters.get(0).display().get("remaining_kwh"));assertFalse(meters.get(0).display().containsKey("gatherCode"));
        assertFalse(meters.get(0).display().containsKey("userId"));
    }
    @Test public void missingOrDuplicateBoundIdentifiersRejectRatherThanGuess() throws Exception {
        Map<String,Object> meter=row("meterType","1","gatherCode","synthetic-owned-meter","remainingPower",10);
        rejects(()->WanliData.meters(Arrays.asList(meter,meter)));
        rejects(()->WanliData.meters(Collections.singletonList(row("meterType","1","remainingPower",10))));
    }
    @Test public void partialReadingRemainsUnavailable() throws Exception {
        Map<String,Object> d=WanliData.days(row("list",Collections.singletonList(day(1,"1",null,"0"))),"2026-10").get(0);
        assertEquals("NO_READING",d.get("state"));assertNull(d.get("kwh"));
    }
    private interface InvalidAction { void run() throws Exception; }
    private void rejects(InvalidAction action) throws Exception {
        try {action.run();fail("Expected invalid upstream data");}catch(WanliData.Invalid expected){assertEquals("UPSTREAM_DATA_INVALID",expected.getMessage());}
    }
}
