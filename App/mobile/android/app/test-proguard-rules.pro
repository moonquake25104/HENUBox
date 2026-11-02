# Error Prone annotations reference the host JDK compiler model. It is never used
# by AndroidX instrumentation at runtime and is absent from Android's bootclasspath.
-dontwarn javax.lang.model.element.Modifier
