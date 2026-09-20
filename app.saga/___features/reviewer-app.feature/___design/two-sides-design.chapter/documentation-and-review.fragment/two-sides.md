# The two sides of the reviewer {#two-sides}

The header, not the sidebar, carries the distinction between what the
application is and how it is changing.

## Documentation is what the app is {#documentation-side}

Documentation answers "what is this?" — the overview, then the feature set,
each feature with its stories, design, quality, and implementation deck. Its
sidebar holds exactly two sections, Overview and Features.

Reviews are deliberately absent from it. A sidebar row for them would offer the
same destination twice and imply that reviews are part of what the application
*is*, when they are the record of how it came to be.

## Review is how it is changing {#review-side}

Review answers "what is changing?" — the pull-request reviews, and the views
that only mean something against a comparison: Code Diff, Coverage, and Change.
A diff appears where a change is being reviewed, and nowhere else.

One flag on the page decides which side the reader is on, so no page can
belong to both.

## Where the two sides meet {#related-reviews}

A feature, story, or acceptance criterion lists the reviews that changed the
code it explains.

Nobody writes that list. It is computed by intersecting the code each record
references with each review's changed lines, following the chain that already
reaches code: a criterion through its design or a slide Item, a story through
its criteria, a feature through its stories. There is no new record and no new
relation, so there is nothing for an author or an agent to keep up to date, and
nothing anyone can inflate.

That is also why the page says the list was derived. A reader who mistook it
for a link someone made would draw the wrong conclusion from its absence: an
empty list means the reviews touched none of this code, not that somebody
forgot.

It is a footnote, not a headline. The record's own content comes first.
