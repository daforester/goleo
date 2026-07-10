package {{.PackageName}};

import android.Manifest;
import android.annotation.SuppressLint;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.bluetooth.BluetoothAdapter;
import android.bluetooth.BluetoothDevice;
import android.bluetooth.BluetoothGatt;
import android.bluetooth.BluetoothGattCallback;
import android.bluetooth.BluetoothGattCharacteristic;
import android.bluetooth.BluetoothGattService;
import android.bluetooth.BluetoothManager;
import android.bluetooth.BluetoothProfile;
import android.bluetooth.le.BluetoothLeScanner;
import android.bluetooth.le.ScanCallback;
import android.bluetooth.le.ScanResult;
import android.content.ClipData;
import android.content.ClipboardManager;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.hardware.Sensor;
import android.hardware.SensorEvent;
import android.hardware.SensorEventListener;
import android.hardware.SensorManager;
import android.net.Uri;
import android.nfc.NdefMessage;
import android.nfc.NdefRecord;
import android.nfc.NfcAdapter;
import android.nfc.Tag;
import android.nfc.tech.Ndef;
import android.os.BatteryManager;
import android.os.Build;
import android.os.Bundle;
import android.util.Base64;
import android.view.WindowManager;
import android.webkit.GeolocationPermissions;
import android.webkit.PermissionRequest;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.webkit.WebSettings;
import android.webkit.WebResourceRequest;
import androidx.activity.result.ActivityResultLauncher;
import androidx.activity.result.contract.ActivityResultContracts;
import androidx.appcompat.app.AppCompatActivity;
import androidx.core.app.ActivityCompat;
import androidx.core.app.NotificationCompat;
import androidx.core.app.NotificationManagerCompat;
import androidx.core.content.ContextCompat;
import androidx.work.Constraints;
import androidx.work.Data;
import androidx.work.ExistingWorkPolicy;
import androidx.work.NetworkType;
import androidx.work.OneTimeWorkRequest;
import androidx.work.WorkManager;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import gomobile.BackgroundProvider;
import gomobile.BatteryProvider;
import gomobile.BLEProvider;
import gomobile.ClipboardProvider;
import gomobile.Gomobile;
import gomobile.NFCProvider;
import gomobile.Notifier;
import gomobile.SensorsProvider;
import gomobile.WakeLockProvider;

public class MainActivity extends AppCompatActivity {
    private static final String NOTIFICATION_CHANNEL_ID = "goleo_default";
    private static final int NOTIFICATION_PERMISSION_REQUEST = 9842;
    private static final int WEB_PERMISSION_REQUEST = 9843;
    private static final int GEO_PERMISSION_REQUEST = 9844;
    private static final int BLE_PERMISSION_REQUEST = 9845;
    private static final AtomicInteger notificationId = new AtomicInteger(1);

    private WebView webView;
    private PermissionRequest pendingWebPermission;
    private String pendingGeoOrigin;
    private GeolocationPermissions.Callback pendingGeoCallback;
    private ValueCallback<Uri[]> pendingFileChooser;
    private NfcAdapter nfcAdapter;
    private PendingIntent nfcPendingIntent;
    private final GoleoNfc goleoNfc = new GoleoNfc();

    // Must be registered before onStart (a field initializer guarantees
    // that), so <input type="file"> in the WebView's browser-API fallbacks
    // (see DialogsDemo.vue) can show a real system file picker.
    private final ActivityResultLauncher<Intent> fileChooserLauncher = registerForActivityResult(
            new ActivityResultContracts.StartActivityForResult(), result -> {
                if (pendingFileChooser == null) {
                    return;
                }
                Uri[] uris = null;
                Intent data = result.getData();
                if (result.getResultCode() == RESULT_OK && data != null) {
                    if (data.getClipData() != null) {
                        int count = data.getClipData().getItemCount();
                        uris = new Uri[count];
                        for (int i = 0; i < count; i++) {
                            uris[i] = data.getClipData().getItemAt(i).getUri();
                        }
                    } else if (data.getData() != null) {
                        uris = new Uri[]{data.getData()};
                    }
                }
                pendingFileChooser.onReceiveValue(uris);
                pendingFileChooser = null;
            });

    @SuppressLint("SetJavaScriptEnabled")
    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        createNotificationChannel();
        // Go's os.UserConfigDir/os.UserHomeDir (used by the FS feature's
        // AppDataDir/HomeDir) need $HOME, which the gomobile host process
        // never sets on its own — must run before startServer.
        Gomobile.setHomeDir(getFilesDir().getAbsolutePath());
        Gomobile.setNotifier(new GoleoNotifier());
        Gomobile.setBatteryProvider(new GoleoBattery());
        Gomobile.setWakeLockProvider(new GoleoWakeLock());
        Gomobile.setSensorsProvider(new GoleoSensors());
        Gomobile.setBackgroundProvider(new GoleoBackground());
        Gomobile.setNFCProvider(goleoNfc);
        Gomobile.setBLEProvider(new GoleoBle());
        Gomobile.setClipboardProvider(new GoleoClipboard());

        nfcAdapter = NfcAdapter.getDefaultAdapter(this);
        if (nfcAdapter != null) {
            Intent nfcIntent = new Intent(this, getClass()).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP);
            nfcPendingIntent = PendingIntent.getActivity(this, 0, nfcIntent, PendingIntent.FLAG_MUTABLE);
        }

        Gomobile.startServer(true);

        webView = new WebView(this);
        WebSettings settings = webView.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setAllowFileAccess(true);
        settings.setLoadWithOverviewMode(true);
        settings.setUseWideViewPort(true);
        settings.setCacheMode(WebSettings.LOAD_NO_CACHE);
        settings.setGeolocationEnabled(true);
        settings.setMediaPlaybackRequiresUserGesture(false);

        webView.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return false;
            }
        });

        // Bridge web permission prompts (getUserMedia, navigator.geolocation)
        // to Android runtime permissions so the browser-API fallbacks work.
        // Permissions not declared in the manifest are denied automatically;
        // run `goleo scan` to see which manifest entries your app needs.
        webView.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onPermissionRequest(PermissionRequest request) {
                if (!request.getOrigin().toString().startsWith("http://10.0.2.2")) {
                    request.deny();
                    return;
                }
                List<String> needed = new ArrayList<>();
                for (String resource : request.getResources()) {
                    if (PermissionRequest.RESOURCE_VIDEO_CAPTURE.equals(resource)
                            && !hasPermission(Manifest.permission.CAMERA)) {
                        needed.add(Manifest.permission.CAMERA);
                    } else if (PermissionRequest.RESOURCE_AUDIO_CAPTURE.equals(resource)
                            && !hasPermission(Manifest.permission.RECORD_AUDIO)) {
                        needed.add(Manifest.permission.RECORD_AUDIO);
                    }
                }
                if (needed.isEmpty()) {
                    request.grant(request.getResources());
                    return;
                }
                pendingWebPermission = request;
                ActivityCompat.requestPermissions(MainActivity.this,
                        needed.toArray(new String[0]), WEB_PERMISSION_REQUEST);
            }

            @Override
            public void onGeolocationPermissionsShowPrompt(String origin,
                    GeolocationPermissions.Callback callback) {
                if (hasPermission(Manifest.permission.ACCESS_FINE_LOCATION)
                        || hasPermission(Manifest.permission.ACCESS_COARSE_LOCATION)) {
                    callback.invoke(origin, true, false);
                    return;
                }
                pendingGeoOrigin = origin;
                pendingGeoCallback = callback;
                ActivityCompat.requestPermissions(MainActivity.this,
                        new String[]{Manifest.permission.ACCESS_FINE_LOCATION,
                                Manifest.permission.ACCESS_COARSE_LOCATION},
                        GEO_PERMISSION_REQUEST);
            }

            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> callback,
                    FileChooserParams params) {
                pendingFileChooser = callback;
                try {
                    fileChooserLauncher.launch(params.createIntent());
                } catch (Exception e) {
                    pendingFileChooser = null;
                    return false;
                }
                return true;
            }
        });

        webView.loadUrl("http://10.0.2.2:{{.DevPort}}");
        setContentView(webView);
    }

    @Override
    protected void onResume() {
        super.onResume();
        if (nfcAdapter != null) {
            nfcAdapter.enableForegroundDispatch(this, nfcPendingIntent, null, null);
        }
    }

    @Override
    protected void onPause() {
        if (nfcAdapter != null) {
            nfcAdapter.disableForegroundDispatch(this);
        }
        super.onPause();
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        Tag tag = intent.getParcelableExtra(NfcAdapter.EXTRA_TAG);
        if (tag == null) {
            return;
        }
        if (goleoNfc.pendingWrite != null) {
            NdefMessage toWrite = goleoNfc.pendingWrite;
            goleoNfc.pendingWrite = null;
            try {
                Ndef ndef = Ndef.get(tag);
                if (ndef != null) {
                    ndef.connect();
                    ndef.writeNdefMessage(toWrite);
                    ndef.close();
                }
            } catch (Exception e) {
                // Best-effort: the demo's write() call already returned by
                // the time a tag is tapped, so there's no promise left to
                // reject — a failed write just means try tapping again.
            }
        } else if (goleoNfc.scanning) {
            StringBuilder sb = new StringBuilder();
            for (byte b : tag.getId()) {
                sb.append(String.format("%02x", b));
            }
            Gomobile.emitNFCTag(sb.toString());
        }
    }

    @Override
    protected void onDestroy() {
        Gomobile.setNotifier(null);
        Gomobile.setBatteryProvider(null);
        Gomobile.setWakeLockProvider(null);
        Gomobile.setSensorsProvider(null);
        Gomobile.setBackgroundProvider(null);
        Gomobile.setNFCProvider(null);
        Gomobile.setBLEProvider(null);
        Gomobile.setClipboardProvider(null);
        Gomobile.stopServer();
        super.onDestroy();
    }

    private boolean hasPermission(String permission) {
        return ContextCompat.checkSelfPermission(this, permission)
                == PackageManager.PERMISSION_GRANTED;
    }

    @Override
    public void onRequestPermissionsResult(int requestCode, String[] permissions,
            int[] grantResults) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults);
        if (requestCode == WEB_PERMISSION_REQUEST && pendingWebPermission != null) {
            boolean allGranted = grantResults.length > 0;
            for (int result : grantResults) {
                if (result != PackageManager.PERMISSION_GRANTED) {
                    allGranted = false;
                    break;
                }
            }
            if (allGranted) {
                pendingWebPermission.grant(pendingWebPermission.getResources());
            } else {
                pendingWebPermission.deny();
            }
            pendingWebPermission = null;
        } else if (requestCode == GEO_PERMISSION_REQUEST && pendingGeoCallback != null) {
            boolean granted = false;
            for (int result : grantResults) {
                if (result == PackageManager.PERMISSION_GRANTED) {
                    granted = true;
                    break;
                }
            }
            pendingGeoCallback.invoke(pendingGeoOrigin, granted, false);
            pendingGeoCallback = null;
            pendingGeoOrigin = null;
        }
    }

    private void createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            NotificationChannel channel = new NotificationChannel(
                    NOTIFICATION_CHANNEL_ID,
                    "Notifications",
                    NotificationManager.IMPORTANCE_HIGH);
            getSystemService(NotificationManager.class).createNotificationChannel(channel);
        }
    }

    private class GoleoNotifier implements Notifier {
        @Override
        public void show(String title, String body) {
            if (!permissionGranted()) {
                return;
            }
            NotificationCompat.Builder builder =
                    new NotificationCompat.Builder(MainActivity.this, NOTIFICATION_CHANNEL_ID)
                            .setSmallIcon(android.R.drawable.ic_dialog_info)
                            .setContentTitle(title)
                            .setContentText(body)
                            .setStyle(new NotificationCompat.BigTextStyle().bigText(body))
                            .setPriority(NotificationCompat.PRIORITY_HIGH)
                            .setAutoCancel(true);
            NotificationManagerCompat.from(MainActivity.this)
                    .notify(notificationId.getAndIncrement(), builder.build());
        }

        @Override
        public boolean permissionGranted() {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                return ContextCompat.checkSelfPermission(MainActivity.this,
                        Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED;
            }
            return NotificationManagerCompat.from(MainActivity.this).areNotificationsEnabled();
        }

        @Override
        public String requestPermission() {
            if (permissionGranted()) {
                return "granted";
            }
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                runOnUiThread(() -> ActivityCompat.requestPermissions(MainActivity.this,
                        new String[]{Manifest.permission.POST_NOTIFICATIONS},
                        NOTIFICATION_PERMISSION_REQUEST));
                return "default";
            }
            return "denied";
        }
    }

    // registerReceiver(null, filter) is the standard Android idiom for
    // reading the last sticky ACTION_BATTERY_CHANGED broadcast synchronously,
    // without registering a persistent listener. No permission required.
    private class GoleoBattery implements BatteryProvider {
        private Intent batteryIntent() {
            return registerReceiver(null, new android.content.IntentFilter(Intent.ACTION_BATTERY_CHANGED));
        }

        @Override
        public double level() {
            Intent i = batteryIntent();
            if (i == null) return -1;
            int level = i.getIntExtra(BatteryManager.EXTRA_LEVEL, -1);
            int scale = i.getIntExtra(BatteryManager.EXTRA_SCALE, -1);
            if (level < 0 || scale <= 0) return -1;
            return (double) level / scale;
        }

        @Override
        public boolean charging() {
            Intent i = batteryIntent();
            if (i == null) return false;
            int status = i.getIntExtra(BatteryManager.EXTRA_STATUS, -1);
            return status == BatteryManager.BATTERY_STATUS_CHARGING
                    || status == BatteryManager.BATTERY_STATUS_FULL;
        }

        // Android has no public API for time-to-full/time-to-empty estimates.
        @Override
        public double chargingTime() { return -1; }

        @Override
        public double dischargingTime() { return -1; }
    }

    // FLAG_KEEP_SCREEN_ON needs no permission and no wakelock service — it's
    // a window flag the OS honors only while this Activity is in front.
    private class GoleoClipboard implements ClipboardProvider {
        @Override
        public String readText() {
            final String[] result = {""};
            final CountDownLatch latch = new CountDownLatch(1);
            runOnUiThread(() -> {
                try {
                    ClipboardManager cm = getSystemService(ClipboardManager.class);
                    if (cm != null && cm.hasPrimaryClip() && cm.getPrimaryClip().getItemCount() > 0) {
                        CharSequence text = cm.getPrimaryClip().getItemAt(0).coerceToText(MainActivity.this);
                        if (text != null) result[0] = text.toString();
                    }
                } catch (Exception e) {
                    // leave empty on failure
                } finally {
                    latch.countDown();
                }
            });
            try {
                latch.await(2, java.util.concurrent.TimeUnit.SECONDS);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
            return result[0];
        }

        @Override
        public void writeText(String text) {
            runOnUiThread(() -> {
                try {
                    ClipboardManager cm = getSystemService(ClipboardManager.class);
                    if (cm != null) {
                        cm.setPrimaryClip(ClipData.newPlainText("goleo", text));
                    }
                } catch (Exception e) {
                    // best effort
                }
            });
        }
    }

    private class GoleoWakeLock implements WakeLockProvider {
        @Override
        public void request(String typeName) {
            runOnUiThread(() ->
                    getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON));
        }

        @Override
        public void release() {
            runOnUiThread(() ->
                    getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON));
        }
    }

    // Streams accelerometer/gyroscope/magnetometer readings from
    // SensorManager to the frontend via Gomobile.emitSensorReading, which
    // turns into a goleo:sensorReading event (see SensorsDemo.vue).
    private class GoleoSensors implements SensorsProvider, SensorEventListener {
        private final Map<String, Sensor> active = new HashMap<>();

        private int androidType(String name) {
            switch (name) {
                case "accelerometer": return Sensor.TYPE_ACCELEROMETER;
                case "gyroscope": return Sensor.TYPE_GYROSCOPE;
                case "magnetometer": return Sensor.TYPE_MAGNETIC_FIELD;
                default: return -1;
            }
        }

        @Override
        public void startSensor(String sensorType) throws Exception {
            SensorManager sm = getSystemService(SensorManager.class);
            int type = androidType(sensorType);
            Sensor sensor = type < 0 ? null : sm.getDefaultSensor(type);
            if (sensor == null) {
                throw new Exception("sensor not available on this device: " + sensorType);
            }
            active.put(sensorType, sensor);
            sm.registerListener(this, sensor, SensorManager.SENSOR_DELAY_UI);
        }

        @Override
        public void stopSensor(String sensorType) {
            Sensor sensor = active.remove(sensorType);
            if (sensor != null) {
                getSystemService(SensorManager.class).unregisterListener(this, sensor);
            }
        }

        @Override
        public void onSensorChanged(SensorEvent event) {
            String type;
            switch (event.sensor.getType()) {
                case Sensor.TYPE_ACCELEROMETER: type = "accelerometer"; break;
                case Sensor.TYPE_GYROSCOPE: type = "gyroscope"; break;
                case Sensor.TYPE_MAGNETIC_FIELD: type = "magnetometer"; break;
                default: return;
            }
            float x = event.values.length > 0 ? event.values[0] : 0f;
            float y = event.values.length > 1 ? event.values[1] : 0f;
            float z = event.values.length > 2 ? event.values[2] : 0f;
            Gomobile.emitSensorReading(type, x, y, z, event.timestamp);
        }

        @Override
        public void onAccuracyChanged(Sensor sensor, int accuracy) {}
    }

    // Runs a registered sync tag as a WorkManager OneTimeWorkRequest,
    // deferred until connectivity is available; GoleoSyncWorker.doWork()
    // reports back via Gomobile.emitBackgroundSync when it actually runs.
    private class GoleoBackground implements BackgroundProvider {
        @Override
        public void registerSync(String tag) {
            Data input = new Data.Builder().putString("tag", tag).build();
            Constraints constraints = new Constraints.Builder()
                    .setRequiredNetworkType(NetworkType.CONNECTED)
                    .build();
            OneTimeWorkRequest request = new OneTimeWorkRequest.Builder(GoleoSyncWorker.class)
                    .setInputData(input)
                    .setConstraints(constraints)
                    .build();
            WorkManager.getInstance(MainActivity.this)
                    .enqueueUniqueWork(tag, ExistingWorkPolicy.REPLACE, request);
        }

        // WorkManager needs no runtime permission to schedule work.
        @Override
        public boolean getPermission() { return true; }

        @Override
        public void requestPermission() {}
    }

    // Scanning/writing state read by onNewIntent (NFC discovery only reaches
    // the app there, via foreground dispatch — not through this class).
    private class GoleoNfc implements NFCProvider {
        volatile boolean scanning = false;
        volatile NdefMessage pendingWrite = null;

        @Override
        public void startScan() { scanning = true; }

        @Override
        public void stopScan() { scanning = false; }

        // messageJSON is runtime.NFCMessage marshaled by Go: {"records":
        // [{"type":"text","mediaType":"...","data":"<base64>"}, ...]} — Go's
        // encoding/json base64-encodes []byte fields automatically.
        @Override
        public void writeJSON(String messageJSON) throws Exception {
            JSONObject obj = new JSONObject(messageJSON);
            JSONArray recordsArr = obj.optJSONArray("records");
            List<NdefRecord> records = new ArrayList<>();
            if (recordsArr != null) {
                for (int i = 0; i < recordsArr.length(); i++) {
                    JSONObject rec = recordsArr.getJSONObject(i);
                    String type = rec.optString("type", "text");
                    String mediaType = rec.optString("mediaType", "text/plain");
                    byte[] data = Base64.decode(rec.optString("data", ""), Base64.DEFAULT);
                    if ("text".equals(type)) {
                        records.add(NdefRecord.createTextRecord("en", new String(data, "UTF-8")));
                    } else {
                        records.add(NdefRecord.createMime(mediaType, data));
                    }
                }
            }
            pendingWrite = new NdefMessage(records.toArray(new NdefRecord[0]));
        }
    }

    // BLE read/write must be serialized per connection — BluetoothGatt
    // rejects a new operation while one is pending — so each connected
    // device gets one persistent callback tracking whichever operation
    // (connect, read, or write) is currently outstanding via opLatch.
    private static class GattSession {
        BluetoothGatt gatt;
        volatile boolean servicesReady = false;
        volatile CountDownLatch connectLatch;
        volatile CountDownLatch opLatch;
        volatile byte[] readResult;
        volatile boolean writeOk;

        final BluetoothGattCallback callback = new BluetoothGattCallback() {
            @Override
            public void onConnectionStateChange(BluetoothGatt g, int status, int newState) {
                if (newState == BluetoothProfile.STATE_CONNECTED) {
                    g.discoverServices();
                } else {
                    servicesReady = false;
                    if (connectLatch != null) connectLatch.countDown();
                }
            }

            @Override
            public void onServicesDiscovered(BluetoothGatt g, int status) {
                servicesReady = status == BluetoothGatt.GATT_SUCCESS;
                if (connectLatch != null) connectLatch.countDown();
            }

            @SuppressWarnings("deprecation")
            @Override
            public void onCharacteristicRead(BluetoothGatt g, BluetoothGattCharacteristic ch, int status) {
                readResult = status == BluetoothGatt.GATT_SUCCESS ? ch.getValue() : null;
                if (opLatch != null) opLatch.countDown();
            }

            @Override
            public void onCharacteristicWrite(BluetoothGatt g, BluetoothGattCharacteristic ch, int status) {
                writeOk = status == BluetoothGatt.GATT_SUCCESS;
                if (opLatch != null) opLatch.countDown();
            }
        };
    }

    // Scan discovers ambient devices (no pairing needed for BLE advertising);
    // connect/read/write need a real GATT peripheral exposing the given
    // service/characteristic UUIDs to fully exercise.
    private class GoleoBle implements BLEProvider {
        private final Map<String, BluetoothDevice> discovered = new ConcurrentHashMap<>();
        private final Map<String, GattSession> sessions = new ConcurrentHashMap<>();

        private boolean hasBlePermission() {
            if (Build.VERSION.SDK_INT >= 31) {
                return hasPermission(Manifest.permission.BLUETOOTH_SCAN)
                        && hasPermission(Manifest.permission.BLUETOOTH_CONNECT);
            }
            return hasPermission(Manifest.permission.ACCESS_FINE_LOCATION);
        }

        // Returns a JSON-encoded runtime.BLEDevice ({"id":...,"name":...,
        // "rssi":...}) rather than a BLEDevice object: gobind cannot
        // generate a reverse-bound (Java-implemented) proxy method that
        // returns a pointer to a struct defined in a different package
        // than the Go interface — confirmed via an isolated gomobile bind
        // run, where the generated proxy silently omitted the method.
        @Override
        public String requestDeviceJSON(String filtersJSON) throws Exception {
            if (!hasBlePermission()) {
                String[] perms = Build.VERSION.SDK_INT >= 31
                        ? new String[]{Manifest.permission.BLUETOOTH_SCAN, Manifest.permission.BLUETOOTH_CONNECT}
                        : new String[]{Manifest.permission.ACCESS_FINE_LOCATION};
                ActivityCompat.requestPermissions(MainActivity.this, perms, BLE_PERMISSION_REQUEST);
                throw new Exception("Bluetooth permission requested — please retry once granted");
            }
            BluetoothManager btManager = getSystemService(BluetoothManager.class);
            BluetoothAdapter adapter = btManager.getAdapter();
            if (adapter == null || !adapter.isEnabled()) {
                throw new Exception("Bluetooth is not enabled");
            }
            BluetoothLeScanner scanner = adapter.getBluetoothLeScanner();
            if (scanner == null) {
                throw new Exception("BLE scanning not supported on this device");
            }

            final CountDownLatch latch = new CountDownLatch(1);
            final JSONObject[] result = new JSONObject[1];
            ScanCallback callback = new ScanCallback() {
                @Override
                public void onScanResult(int callbackType, ScanResult sr) {
                    synchronized (result) {
                        if (result[0] != null) return;
                        BluetoothDevice device = sr.getDevice();
                        discovered.put(device.getAddress(), device);
                        try {
                            JSONObject d = new JSONObject();
                            d.put("id", device.getAddress());
                            d.put("name", device.getName() != null ? device.getName() : "Unknown");
                            d.put("rssi", sr.getRssi());
                            result[0] = d;
                        } catch (JSONException e) {
                            // JSONObject.put only throws for a null key, which
                            // never happens here — nothing to recover from.
                        }
                    }
                    latch.countDown();
                }
            };
            scanner.startScan(callback);
            boolean found = latch.await(10, TimeUnit.SECONDS);
            scanner.stopScan(callback);
            if (!found || result[0] == null) {
                throw new Exception("no BLE device found within 10s");
            }
            return result[0].toString();
        }

        @Override
        public void connect(String deviceID) throws Exception {
            BluetoothDevice device = discovered.get(deviceID);
            if (device == null) {
                throw new Exception("unknown device (call requestDevice first): " + deviceID);
            }
            GattSession session = new GattSession();
            session.connectLatch = new CountDownLatch(1);
            session.gatt = device.connectGatt(MainActivity.this, false, session.callback);
            sessions.put(deviceID, session);
            boolean done = session.connectLatch.await(10, TimeUnit.SECONDS);
            if (!done || !session.servicesReady) {
                sessions.remove(deviceID);
                session.gatt.close();
                throw new Exception("failed to connect to " + deviceID);
            }
        }

        @Override
        public void disconnect(String deviceID) {
            GattSession session = sessions.remove(deviceID);
            if (session != null && session.gatt != null) {
                session.gatt.disconnect();
                session.gatt.close();
            }
        }

        @SuppressWarnings("deprecation")
        @Override
        public byte[] read(String deviceID, String service, String characteristic) throws Exception {
            GattSession session = sessions.get(deviceID);
            if (session == null) {
                throw new Exception("not connected: " + deviceID);
            }
            BluetoothGattCharacteristic ch = findCharacteristic(session.gatt, service, characteristic);
            session.opLatch = new CountDownLatch(1);
            session.readResult = null;
            if (!session.gatt.readCharacteristic(ch)) {
                throw new Exception("readCharacteristic failed to start");
            }
            if (!session.opLatch.await(10, TimeUnit.SECONDS)) {
                throw new Exception("read timed out");
            }
            if (session.readResult == null) {
                throw new Exception("read failed");
            }
            return session.readResult;
        }

        @SuppressWarnings("deprecation")
        @Override
        public void write(String deviceID, String service, String characteristic, byte[] data) throws Exception {
            GattSession session = sessions.get(deviceID);
            if (session == null) {
                throw new Exception("not connected: " + deviceID);
            }
            BluetoothGattCharacteristic ch = findCharacteristic(session.gatt, service, characteristic);
            ch.setValue(data);
            session.opLatch = new CountDownLatch(1);
            session.writeOk = false;
            if (!session.gatt.writeCharacteristic(ch)) {
                throw new Exception("writeCharacteristic failed to start");
            }
            if (!session.opLatch.await(10, TimeUnit.SECONDS)) {
                throw new Exception("write timed out");
            }
            if (!session.writeOk) {
                throw new Exception("write failed");
            }
        }

        private BluetoothGattCharacteristic findCharacteristic(BluetoothGatt gatt, String service, String characteristic) throws Exception {
            BluetoothGattService svc = gatt.getService(UUID.fromString(service));
            if (svc == null) {
                throw new Exception("service not found: " + service);
            }
            BluetoothGattCharacteristic ch = svc.getCharacteristic(UUID.fromString(characteristic));
            if (ch == null) {
                throw new Exception("characteristic not found: " + characteristic);
            }
            return ch;
        }
    }
}
