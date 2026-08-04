plugins {
    id("com.android.application")
}

android {
    namespace = "{{.PackageName}}"
    // These MUST track goleo.json the same way the release template does. They were
    // hardcoded 36/24/36 while the release project used mobile.android.{min_sdk,target_sdk},
    // so a project raising min_sdk above 24 failed only on the `goleo emulate` path:
    // gomobile builds the AAR against the configured minimum, and Gradle rejects a library
    // whose minSdk exceeds the app's ("cannot be smaller than version N declared in
    // library"). Below 24 it was the quieter kind of wrong — dev ran on devices the
    // release build did not support.
    compileSdk = {{.TargetSDK}}
    defaultConfig {
        applicationId = "{{.PackageName}}"
        minSdk = {{.MinSDK}}
        targetSdk = {{.TargetSDK}}
        // A dev build is never uploaded, so versionCode stays 1; versionName carries the
        // real version so "what am I running" has an answer on the device.
        versionCode = 1
        versionName = "{{.VersionName}}-dev"
    }
    buildTypes {
        release {
            isMinifyEnabled = false
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
}

dependencies {
    implementation(fileTree(mapOf("dir" to "libs", "include" to listOf("*.aar"))))
    // Align kotlin-stdlib-jdk7/jdk8 with the stdlib version pulled by androidx
    // (they merged into kotlin-stdlib in 1.8; mixed versions cause duplicate classes)
    implementation(platform("org.jetbrains.kotlin:kotlin-bom:1.8.22"))
    implementation("androidx.appcompat:appcompat:1.6.1")
    implementation("androidx.core:core:1.13.1")
    implementation("androidx.webkit:webkit:1.9.0")
    implementation("androidx.work:work-runtime:2.9.0")
}
