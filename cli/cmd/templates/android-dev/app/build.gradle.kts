plugins {
    id("com.android.application")
}

android {
    namespace = "{{.PackageName}}"
    compileSdk = 36
    defaultConfig {
        applicationId = "{{.PackageName}}"
        minSdk = 24
        targetSdk = 36
        versionCode = 1
        versionName = "1.0-dev"
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
}
