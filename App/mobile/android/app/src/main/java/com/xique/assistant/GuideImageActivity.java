package com.xique.assistant;

import android.app.*;
import android.os.*;
import android.graphics.*;
import android.view.*;
import android.widget.*;
import androidx.appcompat.app.AppCompatActivity;
import androidx.core.view.*;
import java.io.File;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Shows an overview and decodes a bounded visible region for detail. */
public final class GuideImageActivity extends AppCompatActivity {
    private ZoomImageView image;
    @Override protected void onCreate(Bundle state){super.onCreate(state);try{
        // API 37 needs its DecorView created before obtaining the insets controller.
        getWindow().getDecorView();
        File file=new File(getIntent().getStringExtra("path")),root=new GuideResources(this).directory;
        if(!file.isFile()||!file.getCanonicalPath().startsWith(root.getCanonicalPath()+File.separator))throw new IllegalArgumentException();
        boolean dark=getSharedPreferences("xique-appearance",MODE_PRIVATE).getString("theme","light").equals("dark");int bg=dark?0xff1a1d24:0xfff7f8fc,ink=dark?0xffeef1f7:0xff252932;
        LinearLayout layout=new LinearLayout(this);layout.setOrientation(1);layout.setBackgroundColor(bg);WindowCompat.setDecorFitsSystemWindows(getWindow(),false);ViewCompat.setOnApplyWindowInsetsListener(layout,(v,insets)->{androidx.core.graphics.Insets b=insets.getInsets(WindowInsetsCompat.Type.systemBars()|WindowInsetsCompat.Type.displayCutout());v.setPadding(b.left,b.top,b.right,b.bottom);return insets;});WindowInsetsControllerCompat bars=WindowCompat.getInsetsController(getWindow(),layout);bars.setAppearanceLightStatusBars(!dark);bars.setAppearanceLightNavigationBars(!dark);getWindow().setStatusBarColor(bg);getWindow().setNavigationBarColor(bg);
        LinearLayout top=new LinearLayout(this);top.setGravity(Gravity.CENTER_VERTICAL);Button close=new Button(this);close.setText("返回");close.setContentDescription("返回校园指南");close.setOnClickListener(v->finish());top.addView(close,new LinearLayout.LayoutParams(dp(68),dp(48)));TextView title=new TextView(this);title.setText(getIntent().getStringExtra("title"));title.setTextColor(ink);title.setTextSize(16);title.setMaxLines(2);top.addView(title,new LinearLayout.LayoutParams(0,-2,1));Button reset=new Button(this);reset.setText("复位");reset.setOnClickListener(v->image.reset());top.addView(reset,new LinearLayout.LayoutParams(dp(68),dp(48)));layout.addView(top);
        image=new ZoomImageView(file,bg);layout.addView(image,new LinearLayout.LayoutParams(-1,0,1));TextView hint=new TextView(this);hint.setText("双指缩放 · 拖动查看 · 双击放大");hint.setTextColor(ink);hint.setGravity(Gravity.CENTER);hint.setPadding(0,dp(10),0,dp(10));layout.addView(hint);setContentView(layout);
    }catch(Exception e){android.util.Log.e("HENUBoxGuide","Image viewer initialization failed",e);Toast.makeText(this,"原图暂时无法打开，请返回重试。",Toast.LENGTH_LONG).show();finish();}}
    private int dp(int n){return Math.round(n*getResources().getDisplayMetrics().density);}
    final class ZoomImageView extends View {
        // One displayed region and one replacement can coexist during decoding.
        static final int DETAIL_PIXELS = 1024 * 1024;
        private static final int DETAIL_DELAY_MS = 120;

        private final ExecutorService worker = Executors.newSingleThreadExecutor();
        private final Paint paint = new Paint(Paint.FILTER_BITMAP_FLAG);
        private final RectF destination = new RectF();
        private final Runnable loadDetail = this::loadVisibleDetail;
        private final ScaleGestureDetector pinch;
        private final GestureDetector drag;
        private BitmapRegionDecoder decoder; // Worker thread only.
        Bitmap preview;
        Bitmap detail;
        private Rect detailRegion;
        private int detailSample, previewSample, width, height, viewportVersion;
        float scale, min;
        private float left, top;
        private boolean decoding;
        volatile boolean ready;
        private volatile boolean closed;

        ZoomImageView(File file, int background) {
            super(GuideImageActivity.this);
            setBackgroundColor(background);
            setContentDescription("校园指南原图，可双指缩放");
            pinch = new ScaleGestureDetector(getContext(), new ScaleGestureDetector.SimpleOnScaleGestureListener() {
                @Override public boolean onScale(ScaleGestureDetector detector) {
                    zoom(scale * detector.getScaleFactor(), detector.getFocusX(), detector.getFocusY());
                    return true;
                }

                @Override public void onScaleEnd(ScaleGestureDetector detector) {
                    scheduleDetail();
                }
            });
            drag = new GestureDetector(getContext(), new GestureDetector.SimpleOnGestureListener() {
                @Override public boolean onDown(MotionEvent event) {
                    return true;
                }

                @Override public boolean onScroll(MotionEvent start, MotionEvent event, float x, float y) {
                    if (ready && !pinch.isInProgress() && event.getPointerCount() == 1) {
                        left -= x;
                        top -= y;
                        bound();
                        viewportChanged();
                    }
                    return true;
                }

                @Override public boolean onDoubleTap(MotionEvent event) {
                    zoom(scale > min * 1.5f ? min : Math.min(4, Math.max(min * 3, 1)), event.getX(), event.getY());
                    return true;
                }
            });
            worker.execute(() -> loadPreview(file));
        }

        private void loadPreview(File file) {
            try {
                decoder = BitmapRegionDecoder.newInstance(file.getPath(), false);
                int sourceWidth = decoder.getWidth();
                int sourceHeight = decoder.getHeight();
                BitmapFactory.Options options = new BitmapFactory.Options();
                options.inSampleSize = 1;
                while (Math.max(sourceWidth, sourceHeight) / options.inSampleSize > 1280) {
                    options.inSampleSize *= 2;
                }
                Bitmap bitmap = BitmapFactory.decodeFile(file.getPath(), options);
                if (bitmap == null) throw new IllegalStateException("Missing image preview");
                post(() -> {
                    if (closed) return;
                    preview = bitmap;
                    previewSample = options.inSampleSize;
                    width = sourceWidth;
                    height = sourceHeight;
                    ready = true;
                    reset();
                });
            } catch (Exception error) {
                android.util.Log.e("HENUBoxGuide", "Image decoder initialization failed", error);
                post(() -> {
                    if (closed) return;
                    Toast.makeText(getContext(), "无法读取原图", Toast.LENGTH_LONG).show();
                    finish();
                });
            }
        }

        void reset() {
            if (!ready || getWidth() == 0 || getHeight() == 0) return;
            min = Math.min((float) getWidth() / width, (float) getHeight() / height);
            scale = min;
            left = (getWidth() - width * scale) / 2;
            top = (getHeight() - height * scale) / 2;
            viewportChanged();
        }

        @Override protected void onSizeChanged(int w, int h, int oldWidth, int oldHeight) {
            reset();
        }

        void zoom(float next, float x, float y) {
            if (scale == 0) return;
            float previous = scale;
            scale = Math.max(min, Math.min(Math.max(4, min), next));
            if (scale == previous) return;
            left = x - (x - left) * scale / previous;
            top = y - (y - top) * scale / previous;
            bound();
            viewportChanged();
        }

        private void bound() {
            left = width * scale <= getWidth() ? (getWidth() - width * scale) / 2 : Math.max(getWidth() - width * scale, Math.min(0, left));
            top = height * scale <= getHeight() ? (getHeight() - height * scale) / 2 : Math.max(getHeight() - height * scale, Math.min(0, top));
        }

        @Override public boolean onTouchEvent(MotionEvent event) {
            getParent().requestDisallowInterceptTouchEvent(true);
            pinch.onTouchEvent(event);
            drag.onTouchEvent(event);
            return true;
        }

        private void viewportChanged() {
            viewportVersion++;
            invalidate();
            scheduleDetail();
        }

        private void scheduleDetail() {
            removeCallbacks(loadDetail);
            if (!closed) postDelayed(loadDetail, DETAIL_DELAY_MS);
        }

        private void loadVisibleDetail() {
            if (closed || decoding || scale == 0 || pinch.isInProgress()) return;
            Rect visible = new Rect(
                Math.max(0, (int) Math.floor(-left / scale)),
                Math.max(0, (int) Math.floor(-top / scale)),
                Math.min(width, (int) Math.ceil((getWidth() - left) / scale)),
                Math.min(height, (int) Math.ceil((getHeight() - top) / scale)));
            if (visible.isEmpty()) return;

            int insetX = visible.width() / 8;
            int insetY = visible.height() / 8;
            Rect region = new Rect(Math.max(0, visible.left - insetX), Math.max(0, visible.top - insetY),
                Math.min(width, visible.right + insetX), Math.min(height, visible.bottom + insetY));
            BitmapFactory.Options options = new BitmapFactory.Options();
            options.inSampleSize = Integer.highestOneBit(Math.max(1, (int) (1 / scale)));
            while (((region.width() + options.inSampleSize - 1L) / options.inSampleSize)
                * ((region.height() + options.inSampleSize - 1L) / options.inSampleSize) > DETAIL_PIXELS) {
                options.inSampleSize *= 2;
            }
            if (options.inSampleSize >= previewSample
                || (detailRegion != null && detailSample <= options.inSampleSize && detailRegion.contains(visible))) return;

            // Keep the previous detail visible until one complete replacement is ready.
            int requestedVersion = viewportVersion;
            decoding = true;
            worker.execute(() -> {
                Bitmap bitmap = null;
                try {
                    if (!closed) bitmap = decoder.decodeRegion(region, options);
                } catch (RuntimeException error) {
                    android.util.Log.w("HENUBoxGuide", "Image detail decoding failed", error);
                }
                Bitmap replacement = bitmap;
                post(() -> {
                    decoding = false;
                    if (closed) return;
                    if (requestedVersion != viewportVersion) {
                        scheduleDetail();
                    } else if (replacement != null) {
                        detail = replacement;
                        detailRegion = region;
                        detailSample = options.inSampleSize;
                        invalidate();
                    }
                });
            });
        }

        @Override protected void onDraw(Canvas canvas) {
            super.onDraw(canvas);
            if (preview == null) return;
            drawRegion(canvas, preview, 0, 0, width, height);
            if (detail != null) drawRegion(canvas, detail, detailRegion.left, detailRegion.top, detailRegion.right, detailRegion.bottom);
        }

        private void drawRegion(Canvas canvas, Bitmap bitmap, int x0, int y0, int x1, int y1) {
            destination.set(left + x0 * scale, top + y0 * scale, left + x1 * scale, top + y1 * scale);
            canvas.drawBitmap(bitmap, null, destination, paint);
        }

        @Override protected void onDetachedFromWindow() {
            closed = true;
            removeCallbacks(loadDetail);
            preview = null;
            detail = null;
            worker.execute(() -> {
                if (decoder != null) decoder.recycle();
            });
            worker.shutdown();
            super.onDetachedFromWindow();
        }
    }
}
