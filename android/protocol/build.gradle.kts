// Pure Kotlin/JVM implementation of the PocketPad wire protocol, shared by the
// Android app. Kept free of Android dependencies so it builds and tests anywhere.
plugins {
    kotlin("jvm") version "2.1.0"
}

group = "io.pocketpad"
version = "1.0"

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions { jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17) }
}

dependencies {
    testImplementation(kotlin("test"))
    testImplementation("org.junit.jupiter:junit-jupiter:5.11.4")
    testRuntimeOnly("org.junit.platform:junit-platform-launcher")
    testImplementation("org.json:json:20240303")
}

tasks.test {
    useJUnitPlatform()
    // The vectors live at the repository root, shared with the Go receiver.
    systemProperty("vectors", rootDir.resolve("../../protocol/testvectors/v1.json").absolutePath)
}
