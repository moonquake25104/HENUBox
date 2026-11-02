package com.xique.assistant.focus;

import android.content.Context;
import android.os.SystemClock;
import android.provider.Settings;
import org.json.JSONArray;
import org.json.JSONObject;
import java.time.LocalDate;
import java.time.ZoneId;
import java.util.UUID;

/** All transitions and SQLite access are serialized; WebView and notifications share this engine. */
public final class FocusEngine {
    public interface Clock { long elapsed(); long wall(); int boot(); }
    private static FocusEngine instance;
    private final FocusStore store;
    private final Clock clock;
    private boolean foregroundAttached;
    private long heatmapRevision=-1;
    private String heatmapDay="";
    private JSONObject heatmapCache;
    private static final ZoneId ZONE=ZoneId.of("Asia/Shanghai");
    public static synchronized FocusEngine get(Context context) {
        if(instance==null) {
            Context app=context.getApplicationContext();
            instance=new FocusEngine(app,new Clock() {
                public long elapsed(){return SystemClock.elapsedRealtime();}
                public long wall(){return System.currentTimeMillis();}
                public int boot(){return Settings.Global.getInt(app.getContentResolver(),Settings.Global.BOOT_COUNT,-1);}
            });
        }return instance;
    }
    public FocusEngine(Context context,Clock clock) {store=new FocusStore(context);this.clock=clock;}
    public synchronized void close() { store.close(); }
    private static JSONObject settings() throws Exception {
        return new JSONObject().put("focus_minutes",25).put("short_minutes",5).put("long_minutes",15).put("long_every",4).put("auto_break",false).put("auto_focus",false).put("sound",true).put("vibrate",true);
    }
    private JSONObject load() throws Exception {
        JSONObject s=store.read();
        if(s==null) s=new JSONObject().put("schema_version",1).put("revision",0).put("settings",settings()).put("labels",new JSONArray().put("自习").put("阅读").put("英语")).put("timer",JSONObject.NULL).put("round",0).put("next_mode","focus");
        return s;
    }
    private void save(JSONObject s,JSONObject record) throws Exception {store.write(s,record);}
    private void bump(JSONObject s) throws Exception {s.put("revision",s.getLong("revision")+1);}
    private long duration(JSONObject t) {return FocusClock.bounded(t.optLong("accumulated_ms"),t.optLong("anchor_elapsed"),clock.elapsed(),"RUNNING".equals(t.optString("status")),t.optLong("planned_ms"));}
    private void verify(JSONObject s,long revision) {if(s.optLong("revision")!=revision)throw new IllegalArgumentException("REVISION_CONFLICT");}
    public static String label(String value) {
        String t=value.trim();if(t.codePointCount(0,t.length())>40||t.chars().anyMatch(Character::isISOControl))throw new IllegalArgumentException("INVALID_ARGUMENT");return t;
    }
    private JSONObject begin(JSONObject s,String mode,String label,String id) throws Exception {
        JSONObject p=s.getJSONObject("settings");
        long planned="countup".equals(mode)?86400000L:p.getInt("focus".equals(mode)?"focus_minutes":"short_break".equals(mode)?"short_minutes":"long_minutes")*60000L;
        return new JSONObject().put("id",id).put("mode",mode).put("label",label).put("status","RUNNING").put("started_at",clock.wall()).put("anchor_elapsed",clock.elapsed()).put("boot_count",clock.boot()).put("accumulated_ms",0).put("planned_ms",planned).put("checkpoint_elapsed",clock.elapsed());
    }
    /** At most one completion on restoration. Never invent unattended auto-start rounds. */
    private JSONObject settle(boolean allowAuto) throws Exception {
        JSONObject s=load(),t=s.optJSONObject("timer");if(t==null)return s;
        if(t.optInt("boot_count")!=clock.boot()||clock.elapsed()<t.optLong("anchor_elapsed")) {
            t.put("status","INTERRUPTED").put("anchor_elapsed",clock.elapsed()).put("boot_count",clock.boot());
            s.put("interruption",true);bump(s);save(s,null);return s;
        }
        if("RUNNING".equals(t.optString("status"))&&duration(t)>=t.getLong("planned_ms")) return end(s,true,allowAuto);
        return s;
    }
    private JSONObject end(JSONObject s,boolean natural,boolean allowAuto) throws Exception {
        JSONObject t=s.getJSONObject("timer");String mode=t.getString("mode");long actual=duration(t);
        JSONObject r=new JSONObject().put("id",t.getString("id")).put("mode",mode).put("label",t.getString("label")).put("started_at",t.getLong("started_at")).put("ended_at",clock.wall()).put("actual_ms",actual).put("completed",natural&&"focus".equals(mode));
        int round=s.getInt("round");if(natural&&"focus".equals(mode))round++;
        if(natural&&"long_break".equals(mode))round=0;
        String next=natural?FocusClock.next(mode,round,s.getJSONObject("settings").getInt("long_every")):"focus";
        s.put("timer",JSONObject.NULL).put("round",round).put("next_mode",next).put("interruption",false).put("last_finished",r);
        if(natural) {
            s.put("pending_alert",new JSONObject(r.toString()));
            JSONObject p=s.getJSONObject("settings");
            boolean auto=allowAuto&&!"countup".equals(mode)&&p.getBoolean("focus".equals(next)?"auto_focus":"auto_break");
            if(auto)s.put("timer",begin(s,next,t.getString("label"),UUID.randomUUID().toString()));
        }
        bump(s);save(s,r);return s;
    }
    public synchronized void setForegroundAttached(boolean attached) { foregroundAttached=attached; }
    public synchronized boolean isForegroundAttached() { return foregroundAttached; }
    public synchronized JSONObject getState() throws Exception {return present(settle(foregroundAttached));}
    /** Service clock: snapshot checkpoints don't change the command revision. */
    public synchronized JSONObject tick() throws Exception {
        JSONObject s=settle(true),t=s.optJSONObject("timer");
        if(t!=null&&"RUNNING".equals(t.optString("status"))&&clock.elapsed()-t.optLong("checkpoint_elapsed")>=15000) {
            t.put("accumulated_ms",duration(t)).put("anchor_elapsed",clock.elapsed()).put("checkpoint_elapsed",clock.elapsed());save(s,null);
        }return present(s);
    }
    public synchronized JSONObject takeAlert() throws Exception {
        JSONObject s=load(),alert=s.optJSONObject("pending_alert");if(alert!=null){s.remove("pending_alert");save(s,null);}return alert;
    }
    public synchronized JSONObject command(String action,long revision,String requestId,String mode,String text) throws Exception {
        if(!requestId.matches("[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}"))throw new IllegalArgumentException("INVALID_ARGUMENT");
        JSONObject s=settle(false);JSONObject t=s.optJSONObject("timer");String name=label(text);
        if("start".equals(action)&&t!=null&&requestId.equals(t.getString("id"))) {
            if(!mode.equals(t.getString("mode"))||!name.equals(t.getString("label")))throw new IllegalArgumentException("INVALID_ARGUMENT");
            return present(s);
        }
        verify(s,revision);
        switch(action) {
            case "start":
                if(t!=null)throw new IllegalArgumentException("TIMER_ACTIVE");
                if(!mode.matches("focus|countup|short_break|long_break"))throw new IllegalArgumentException("INVALID_ARGUMENT");
                if(store.recorded(requestId))throw new IllegalArgumentException("OPERATION_FINISHED");
                s.put("timer",begin(s,mode,name,requestId)).put("interruption",false);break;
            case "pause":
                if(t==null||!"RUNNING".equals(t.getString("status")))throw new IllegalArgumentException("INVALID_STATE");
                t.put("accumulated_ms",duration(t)).put("status","PAUSED").put("anchor_elapsed",clock.elapsed());break;
            case "resume":
                if(t==null||"RUNNING".equals(t.getString("status")))throw new IllegalArgumentException("INVALID_STATE");
                t.put("status","RUNNING").put("anchor_elapsed",clock.elapsed()).put("checkpoint_elapsed",clock.elapsed()).put("boot_count",clock.boot());s.put("interruption",false);break;
            case "finish":
                if(t==null)throw new IllegalArgumentException("INVALID_STATE");return present(end(s,false,false));
            case "tag":
            case "archive":
                if(name.isEmpty())throw new IllegalArgumentException("INVALID_ARGUMENT");
                JSONArray old=s.getJSONArray("labels"),out=new JSONArray();boolean exists=false;
                for(int i=0;i<old.length();i++){String v=old.getString(i);if(v.equals(name)){exists=true;if("archive".equals(action))continue;}out.put(v);}
                if("tag".equals(action)&&!exists){if(out.length()>=64)throw new IllegalArgumentException("LABEL_LIMIT");out.put(name);}
                s.put("labels",out);if("tag".equals(action)&&exists||"archive".equals(action)&&!exists)return present(s);break;
            default:throw new IllegalArgumentException("INVALID_ARGUMENT");
        }
        bump(s);save(s,null);return present(s);
    }
    /** Notification controls carry the timer identity so old notifications cannot end a new session. */
    public synchronized void notificationAction(String action,String sessionId) throws Exception {
        JSONObject s=settle(false),t=s.optJSONObject("timer");
        if(t==null||!sessionId.equals(t.optString("id")))return;
        command(action,s.getLong("revision"),UUID.randomUUID().toString(),"","");
    }
    public static void validateSettings(JSONObject p) throws Exception {
        String[] numbers={"focus_minutes","short_minutes","long_minutes","long_every"};
        String[] flags={"auto_break","auto_focus","sound","vibrate"};
        if(p.length()!=8)throw new IllegalArgumentException("INVALID_ARGUMENT");
        for(String k:numbers){Object v=p.get(k);if(!(v instanceof Number)||((Number)v).doubleValue()!=((Number)v).intValue())throw new IllegalArgumentException("INVALID_ARGUMENT");int n=((Number)v).intValue();int max=k.equals("long_every")?12:k.equals("focus_minutes")?180:60;if(n<1||n>max)throw new IllegalArgumentException("INVALID_ARGUMENT");}
        for(String k:flags)if(!(p.get(k) instanceof Boolean))throw new IllegalArgumentException("INVALID_ARGUMENT");
    }
    public synchronized JSONObject saveSettings(long revision,JSONObject prefs) throws Exception {
        validateSettings(prefs);JSONObject s=settle(false);verify(s,revision);
        if(s.optJSONObject("timer")!=null)throw new IllegalArgumentException("TIMER_ACTIVE");
        JSONObject old=s.getJSONObject("settings");boolean same=true;java.util.Iterator<String> keys=prefs.keys();while(keys.hasNext()){String k=keys.next();if(!prefs.get(k).equals(old.get(k)))same=false;}
        if(!same){s.put("settings",new JSONObject(prefs.toString()));bump(s);save(s,null);}return present(s);
    }
    private JSONObject present(JSONObject state) throws Exception {
        JSONObject s=new JSONObject(state.toString());s.remove("pending_alert");JSONObject t=s.optJSONObject("timer");
        if(t!=null){long elapsed=duration(t);t.put("elapsed_ms",elapsed).put("remaining_ms",Math.max(0,t.getLong("planned_ms")-elapsed));}
        LocalDate today=java.time.Instant.ofEpochMilli(clock.wall()).atZone(ZONE).toLocalDate();
        long begin=today.atStartOfDay(ZONE).toInstant().toEpochMilli();s.put("today",store.totals(begin,today.plusDays(1).atStartOfDay(ZONE).toInstant().toEpochMilli()));
        LocalDate monday=today.minusDays(today.getDayOfWeek().getValue()-1);JSONArray week=new JSONArray();
        for(int i=0;i<7;i++){LocalDate day=monday.plusDays(i);week.put(store.totals(day.atStartOfDay(ZONE).toInstant().toEpochMilli(),day.plusDays(1).atStartOfDay(ZONE).toInstant().toEpochMilli()).put("date",day.toString()));}
        long revision=s.getLong("revision");
        if(heatmapCache==null||heatmapRevision!=revision||!heatmapDay.equals(today.toString())) {
            LocalDate first=monday.minusWeeks(11);long cutoff=today.plusDays(1).atStartOfDay(ZONE).toInstant().toEpochMilli();
            JSONArray values=store.daily(first.atStartOfDay(ZONE).toInstant().toEpochMilli(),cutoff);
            java.util.Map<String,JSONObject> byDay=new java.util.HashMap<>();for(int i=0;i<values.length();i++){JSONObject d=values.getJSONObject(i);byDay.put(d.getString("date"),d);}
            JSONArray days=new JSONArray();long seconds=0;
            for(int i=0;i<84;i++){String day=first.plusDays(i).toString();JSONObject d=byDay.get(day);if(d==null)d=new JSONObject().put("date",day).put("seconds",0).put("pomodoros",0);seconds+=d.getLong("seconds");days.put(d);}
            heatmapCache=new JSONObject().put("days",days).put("today",today.toString()).put("from",first.toString()).put("to",first.plusDays(83).toString()).put("total_seconds",seconds);
            heatmapRevision=revision;heatmapDay=today.toString();
        }
        return s.put("week",week).put("records",store.recent()).put("heatmap",new JSONObject(heatmapCache.toString()));
    }
}
