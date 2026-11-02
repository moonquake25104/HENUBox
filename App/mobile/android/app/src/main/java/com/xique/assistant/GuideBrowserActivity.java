package com.xique.assistant;

/** Fixed public guide entry, separate browser process with no native JS interfaces. */
public final class GuideBrowserActivity extends LibraryBrowserActivity {
    private String kind(){String k=getIntent().getStringExtra("kind");return GuideResources.PAGES.containsKey(k==null?"":k)?k:"maps";}
    @Override protected String entryUrl(){return GuideResources.PAGES.get(kind());}
    @Override protected String browserTitle(){return GuideResources.TITLES.get(kind());}
    @Override protected String closeDescription(){return "关闭校园指南网页";}
    @Override protected boolean guideExtras(){return true;}
    // Release the external player when leaving; reopening starts it on demand.
    @Override protected void onStop(){super.onStop();finish();}
}
