# Merge-friendly storage model {#merge-friendly-storage-model}

The Saga is partitioned by durable feature and resource identity. Revisions, lifecycle events, relations, test evidence, and review events are independent records rather than edits to one shared document. Competing heads survive a Git merge as an explicit conflict that the domain loader reports.

Supported writers serialize local mutation, stage complete entities, and publish files atomically. This prevents partial local writes while retaining ordinary Git as the cross-workspace merge and audit layer.
