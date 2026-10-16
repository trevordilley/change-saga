# Keep source and documentation in the intended context {#keep-source-and-documentation-in-the-intended-context}

The recommended layout is one change.saga beside the source checkout. A companion documentation repository is also supported when the source repository is selected explicitly. Stable Saga identity and repository identity keep those contexts distinguishable.

The local CLI reads Git history and files from the selected checkout, while the browser presents the corresponding observation or comparison. Documentation should not drift into a different source repository merely because a command ran from another directory.

No remote application service is required to read a local checkout. The [Infrastructure board](/technical/whiteboards/infrastructure) shows the binary, browser, Git and filesystem boundaries.
