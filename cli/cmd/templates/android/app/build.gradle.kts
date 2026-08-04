plugins {
    id("com.android.application")
}

// Release signing comes from the environment, and is read HERE rather than being
// passed in by the CLI.
//
// Anything the CLI puts on gradle's command line shows up in `ps` output and in
// gradle's own logs, and anything written to gradle.properties lands in a file that
// tends to get committed. System.getenv keeps the keystore password out of both.
//
// When GOLEO_ANDROID_KEYSTORE is unset there is simply no release signingConfig, so
// Gradle produces an unsigned release artifact — the CLI refuses that unless you pass
// --no-sign, because an unsigned release AAB cannot be uploaded anywhere.
// Every value is trimmed. `set VAR=path ` in cmd.exe keeps the trailing space, and
// Windows is exactly where people set these by hand — an untrimmed path failed the
// release build deep inside Gradle with "Trailing char < > at index N", which names
// neither the variable nor the space. A trailing space in a password is worse still: it
// fails authentication with no hint as to why.
val goleoKeystore: String? = System.getenv("GOLEO_ANDROID_KEYSTORE")?.trim()
val goleoStorePass: String? = System.getenv("GOLEO_ANDROID_KEYSTORE_PASSWORD")?.trim()
val goleoKeyAlias: String? = System.getenv("GOLEO_ANDROID_KEY_ALIAS")?.trim()
val goleoKeyPass: String? = System.getenv("GOLEO_ANDROID_KEY_PASSWORD")?.trim()
val goleoSigningEnabled = !goleoKeystore.isNullOrBlank()

android {
    namespace = "{{.PackageName}}"
    compileSdk = {{.TargetSDK}}
    defaultConfig {
        applicationId = "{{.PackageName}}"
        minSdk = {{.MinSDK}}
        targetSdk = {{.TargetSDK}}
        // versionCode must increase on every Play upload; versionName is what users
        // see. Both come from goleo.json (version / mobile.android.version_code).
        // They were hardcoded 1 and "1.0" here, so goleo.json's values were loaded and
        // then thrown away, and every build of every app declared the same version.
        versionCode = {{.VersionCode}}
        versionName = "{{.VersionName}}"
    }

    if (goleoSigningEnabled) {
        signingConfigs {
            create("release") {
                storeFile = file(goleoKeystore!!)
                storePassword = goleoStorePass
                keyAlias = goleoKeyAlias
                keyPassword = goleoKeyPass
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            if (goleoSigningEnabled) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    sourceSets {
        getByName("main") {
            assets.srcDirs("src/main/assets")
        }
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
