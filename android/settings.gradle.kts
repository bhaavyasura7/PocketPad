pluginManagement {
    repositories {
        google()
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "pocketpad-android"

include(":app")

// The protocol library is a standalone build so it stays free of Android
// dependencies; the app consumes it via this composite build.
includeBuild("protocol")
