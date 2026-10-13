plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// Google's published test IDs: they always serve test ads.
val ADMOB_TEST_APP_ID = "ca-app-pub-3940256099942544~3347511713"
val ADMOB_TEST_BANNER_ID = "ca-app-pub-3940256099942544/9214589741"

fun admobSetting(property: String, env: String): String? =
    (findProperty(property) as String?)?.takeIf { it.isNotBlank() }
        ?: System.getenv(env)?.takeIf { it.isNotBlank() }

android {
    namespace = "io.pocketpad.app"
    compileSdk = 36

    defaultConfig {
        applicationId = "io.pocketpad.app"
        minSdk = 28 // BluetoothHidDevice is API 28
        targetSdk = 36
        // Release builds pass these from the git tag: -PversionName=0.1.0 -PversionCode=1
        versionCode = (findProperty("versionCode") as String?)?.toInt() ?: 1
        versionName = (findProperty("versionName") as String?) ?: "1.0"

        // AdMob IDs. Debug builds always use Google's test IDs so taps during
        // development never count as invalid traffic; release builds use the
        // real IDs when they are provided (see docs/PLAY_STORE.md).
        manifestPlaceholders["admobAppId"] = ADMOB_TEST_APP_ID
        buildConfigField("String", "ADMOB_BANNER_ID", "\"$ADMOB_TEST_BANNER_ID\"")
    }

    // Release signing comes from the environment (see docs/DEVELOPMENT.md,
    // "Releasing"); the keystore itself never lives in the repository.
    val keystorePath = System.getenv("POCKETPAD_KEYSTORE_FILE")
    signingConfigs {
        if (keystorePath != null) {
            create("release") {
                storeFile = file(keystorePath)
                storePassword = System.getenv("POCKETPAD_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("POCKETPAD_KEY_ALIAS")
                keyPassword = System.getenv("POCKETPAD_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            val appId = admobSetting("admobAppId", "POCKETPAD_ADMOB_APP_ID")
            val bannerId = admobSetting("admobBannerId", "POCKETPAD_ADMOB_BANNER_ID")
            if (appId != null && bannerId != null) {
                manifestPlaceholders["admobAppId"] = appId
                buildConfigField("String", "ADMOB_BANNER_ID", "\"$bannerId\"")
            }
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            if (keystorePath != null) signingConfig = signingConfigs.getByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    lint {
        abortOnError = true
        warningsAsErrors = false
    }

    packaging {
        resources.excludes += "/META-INF/{AL2.0,LGPL2.1}"
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

dependencies {
    // The shared protocol library (composite build substitutes this).
    implementation("io.pocketpad:protocol:1.0")

    val composeBom = platform("androidx.compose:compose-bom:2024.12.01")
    implementation(composeBom)
    implementation("androidx.activity:activity-compose:1.9.3")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-graphics")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-core")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.7")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")
    implementation("com.journeyapps:zxing-android-embedded:4.3.0")

    // Ads (Settings screen only) and the consent form required in the EEA/UK.
    implementation("com.google.android.gms:play-services-ads:24.4.0")
    implementation("com.google.android.ump:user-messaging-platform:3.2.0")

    debugImplementation("androidx.compose.ui:ui-tooling")

    testImplementation(kotlin("test"))
}
