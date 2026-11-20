# Harbor Watch: an offline example

This is a deliberately authored fictional product, separate from the repository's
self-hosting Saga. It demonstrates a persona, a planned feature and story,
current whiteboard documentation, and a phased Plan. The source repository in
`saga.json` identifies the project maintaining this example.

With the binary from the same release, run from the extracted directory:

```sh
change-saga validate change.saga
change-saga open change.saga
change-saga query plan --saga change.saga --plan arrival-window
```

Explore Technical design → Data flows for the Arrival-window whiteboard,
the Roadmap story, and the Plan's Tasks board. Both Tasks are planned. The review checkpoint is unpassed, so the later
Task is held. The statements describe sample intent; no implementation evidence,
test result or human approval is recorded.

All content needed to validate and browse this example is in the archive. It
contains no previous versions or source-code evidence. Git owns earlier versions
of the example in its maintaining repository. To inspect the real Change Saga
product, its code evidence and historical content, clone that repository and use
its `change.saga` with the compatible reader. Missing historical pins in any Saga
still fail explicitly; this example does not change that rule.
