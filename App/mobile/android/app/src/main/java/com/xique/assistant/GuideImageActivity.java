package com.xique.assistant;

import android.app.*;
import android.os.*;
import android.graphics.*;
import android.view.*;
import android.widget.*;
import androidx.appcompat.app.AppCompatActivity;
import androidx.core.view.*;
import java.io.File;
import java.util.*;
import java.util.concurrent.*;

/** Bounded preview plus visible-region tiles; never decodes a whole campus map. */
public final class GuideImageActivity extends AppCompatActivity {
    private TileView image;
    @Override protected void onCreate(Bundle state){super.onCreate(state);try{
        // API 37 needs its DecorView created before obtaining the insets controller.
        getWindow().getDecorView();
        File file=new File(getIntent().getStringExtra("path")),root=new GuideResources(this).directory;
        if(!file.isFile()||!file.getCanonicalPath().startsWith(root.getCanonicalPath()+File.separator))throw new IllegalArgumentException();
        boolean dark=getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("theme","light").equals("dark");int bg=dark?0xff1a1d24:0xfff7f8fc,ink=dark?0xffeef1f7:0xff252932;
        LinearLayout layout=new LinearLayout(this);layout.setOrientation(1);layout.setBackgroundColor(bg);WindowCompat.setDecorFitsSystemWindows(getWindow(),false);ViewCompat.setOnApplyWindowInsetsListener(layout,(v,insets)->{androidx.core.graphics.Insets b=insets.getInsets(WindowInsetsCompat.Type.systemBars()|WindowInsetsCompat.Type.displayCutout());v.setPadding(b.left,b.top,b.right,b.bottom);return insets;});WindowInsetsControllerCompat bars=WindowCompat.getInsetsController(getWindow(),layout);bars.setAppearanceLightStatusBars(!dark);bars.setAppearanceLightNavigationBars(!dark);getWindow().setStatusBarColor(bg);getWindow().setNavigationBarColor(bg);
        LinearLayout top=new LinearLayout(this);top.setGravity(Gravity.CENTER_VERTICAL);Button close=new Button(this);close.setText("返回");close.setContentDescription("返回校园指南");close.setOnClickListener(v->finish());top.addView(close,new LinearLayout.LayoutParams(dp(68),dp(48)));TextView title=new TextView(this);title.setText(getIntent().getStringExtra("title"));title.setTextColor(ink);title.setTextSize(16);title.setMaxLines(2);top.addView(title,new LinearLayout.LayoutParams(0,-2,1));Button reset=new Button(this);reset.setText("复位");reset.setOnClickListener(v->image.reset());top.addView(reset,new LinearLayout.LayoutParams(dp(68),dp(48)));layout.addView(top);
        image=new TileView(file,bg);layout.addView(image,new LinearLayout.LayoutParams(-1,0,1));TextView hint=new TextView(this);hint.setText("双指缩放 · 拖动查看 · 双击放大");hint.setTextColor(ink);hint.setGravity(Gravity.CENTER);hint.setPadding(0,dp(10),0,dp(10));layout.addView(hint);setContentView(layout);
    }catch(Exception e){android.util.Log.e("HENUBoxGuide","Image viewer initialization failed",e);Toast.makeText(this,"原图暂时无法打开，请返回重试。",Toast.LENGTH_LONG).show();finish();}}
    private int dp(int n){return Math.round(n*getResources().getDisplayMetrics().density);}
    final class TileView extends View {
        final ExecutorService worker=Executors.newSingleThreadExecutor();
        final android.util.LruCache<String,Bitmap> cache=new android.util.LruCache<>(8*1024*1024){@Override protected int sizeOf(String k,Bitmap b){return b.getAllocationByteCount();}};
        final Set<String> pending=new HashSet<>();final Paint paint=new Paint(Paint.ANTI_ALIAS_FLAG|Paint.FILTER_BITMAP_FLAG);
        final ScaleGestureDetector pinch;final GestureDetector drag;BitmapRegionDecoder decoder;Bitmap preview;int width,height;float scale,min,left,top;boolean ready;volatile boolean closed;
        TileView(File f,int bg){super(GuideImageActivity.this);setBackgroundColor(bg);setContentDescription("校园指南原图，可双指缩放");
            pinch=new ScaleGestureDetector(getContext(),new ScaleGestureDetector.SimpleOnScaleGestureListener(){@Override public boolean onScale(ScaleGestureDetector d){zoom(scale*d.getScaleFactor(),d.getFocusX(),d.getFocusY());return true;}});
            drag=new GestureDetector(getContext(),new GestureDetector.SimpleOnGestureListener(){@Override public boolean onDown(android.view.MotionEvent e){return true;}@Override public boolean onScroll(android.view.MotionEvent a,android.view.MotionEvent b,float x,float y){if(!pinch.isInProgress()){left-=x;top-=y;bound();invalidate();}return true;}@Override public boolean onDoubleTap(android.view.MotionEvent e){zoom(scale>min*1.5f?min:Math.min(4,Math.max(min*3,1)),e.getX(),e.getY());return true;}});
            worker.execute(()->{try{BitmapRegionDecoder d=BitmapRegionDecoder.newInstance(f.getPath(),false);BitmapFactory.Options o=new BitmapFactory.Options();int sample=1;while(Math.max(d.getWidth(),d.getHeight())/sample>1280)sample*=2;o.inSampleSize=sample;Bitmap p=BitmapFactory.decodeFile(f.getPath(),o);post(()->{if(closed){d.recycle();if(p!=null)p.recycle();return;}decoder=d;preview=p;width=d.getWidth();height=d.getHeight();ready=true;reset();});}catch(Exception e){android.util.Log.e("HENUBoxGuide","Image decoder initialization failed",e);post(()->{Toast.makeText(getContext(),"无法读取原图",Toast.LENGTH_LONG).show();finish();});}});
        }
        void reset(){if(!ready||getWidth()==0)return;min=Math.min((float)getWidth()/width,(float)getHeight()/height);scale=min;left=(getWidth()-width*scale)/2;top=(getHeight()-height*scale)/2;invalidate();}
        @Override protected void onSizeChanged(int w,int h,int ow,int oh){reset();}
        void zoom(float next,float x,float y){if(!ready)return;float previous=scale;scale=Math.max(min,Math.min(Math.max(4,min),next));left=x-(x-left)*scale/previous;top=y-(y-top)*scale/previous;bound();invalidate();}
        void bound(){left=width*scale<=getWidth()?(getWidth()-width*scale)/2:Math.max(getWidth()-width*scale,Math.min(0,left));top=height*scale<=getHeight()?(getHeight()-height*scale)/2:Math.max(getHeight()-height*scale,Math.min(0,top));}
        @Override public boolean onTouchEvent(android.view.MotionEvent e){getParent().requestDisallowInterceptTouchEvent(true);pinch.onTouchEvent(e);drag.onTouchEvent(e);return true;}
        @Override protected void onDraw(Canvas canvas){super.onDraw(canvas);if(!ready||closed)return;RectF destination=new RectF(left,top,left+width*scale,top+height*scale);if(preview!=null)canvas.drawBitmap(preview,null,destination,paint);
            int sample=Math.max(1,Integer.highestOneBit(Math.max(1,(int)(1/scale)))),step=512*sample;
            int x0=Math.max(0,(int)(-left/scale)/step),y0=Math.max(0,(int)(-top/scale)/step),x1=Math.min((width-1)/step,(int)((getWidth()-left)/scale)/step),y1=Math.min((height-1)/step,(int)((getHeight()-top)/scale)/step);
            for(int y=y0;y<=y1;y++)for(int x=x0;x<=x1;x++){Rect region=new Rect(x*step,y*step,Math.min(width,(x+1)*step),Math.min(height,(y+1)*step));String key=sample+":"+x+":"+y;Bitmap b=cache.get(key);if(b!=null)canvas.drawBitmap(b,null,new RectF(left+region.left*scale,top+region.top*scale,left+region.right*scale,top+region.bottom*scale),paint);else if(pending.size()<24&&pending.add(key)){
                    worker.execute(()->{Bitmap tile=null;try{if(!closed){BitmapFactory.Options o=new BitmapFactory.Options();o.inSampleSize=sample;tile=decoder.decodeRegion(region,o);}}catch(Exception ignored){}Bitmap value=tile;post(()->{pending.remove(key);if(!closed&&value!=null){cache.put(key,value);invalidate();}});});
                }}
        }
        @Override protected void onDetachedFromWindow(){closed=true;cache.evictAll();preview=null;worker.execute(()->{if(decoder!=null){decoder.recycle();decoder=null;}});worker.shutdown();super.onDetachedFromWindow();}
    }
}
