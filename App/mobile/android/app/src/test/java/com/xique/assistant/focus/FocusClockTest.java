package com.xique.assistant.focus;

import org.junit.Test;
import static org.junit.Assert.*;

public class FocusClockTest {
    @Test public void monotonicDurationAndPause(){
        assertEquals(15000,FocusClock.elapsed(5000,10000,20000,true));
        assertEquals(5000,FocusClock.elapsed(5000,10000,2000000,false));
        assertEquals(5000,FocusClock.elapsed(5000,20000,10000,true));
    }
    @Test public void cappedCompletion(){assertEquals(60000,FocusClock.bounded(0,10000,999999,true,60000));}
    @Test public void longBreakCycle(){
        assertEquals("short_break",FocusClock.next("focus",3,4));assertEquals("long_break",FocusClock.next("focus",4,4));assertEquals("focus",FocusClock.next("long_break",4,4));
    }
}
