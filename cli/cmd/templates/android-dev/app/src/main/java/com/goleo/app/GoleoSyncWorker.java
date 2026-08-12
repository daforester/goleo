package {{.PackageName}};

import androidx.annotation.NonNull;
import androidx.work.Worker;
import androidx.work.WorkerParameters;
import android.content.Context;

import gomobile.Gomobile;

// WorkManager instantiates Workers via reflection using this exact
// (Context, WorkerParameters) constructor, so this must be a standalone
// top-level class rather than an inner class of MainActivity.
public class GoleoSyncWorker extends Worker {
    public GoleoSyncWorker(@NonNull Context context, @NonNull WorkerParameters params) {
        super(context, params);
    }

    @NonNull
    @Override
    public Result doWork() {
        String tag = getInputData().getString("tag");
        Gomobile.emitBackgroundSync(tag != null ? tag : "");
        return Result.success();
    }
}
