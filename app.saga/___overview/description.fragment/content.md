With AI, a big change is often fastest to build in one large pull request;
that speed is no reason to lose what the change was meant to do. Change Saga
keeps that intent in Git, beside the code, as one Saga per application.

A Saga documents one application:

- **Overview**: the project's name, elevator pitch, a short description, and
  its terms and vocabulary: the words the team uses that a newcomer would not
  know, each linked to the stories and the exact code that define it.
- **Personas**, a **design system**, an **onboarding deck**, and **feature
  flags**.
- **Features**, the durable areas of the product. Each has the same four parts:
  **Product** (prototypes, and user stories with acceptance criteria),
  **Design** (UX flows, UI references, and technical design), **Quality** (test
  cases and their evidence), and **Implementation** (a slide deck whose visual
  elements reference the exact code they explain).

Every pull request gets a **review**: a slide deck explaining what the change
did and why, which must account for every changed line, and whose slides
reviewers approve one by one.

Code maps back to user stories through designs, specifications, and test cases
rather than hand-written code-to-story links. Every link pins what it relied
on: evidence pins code at a commit and follows it as it moves, and records pin
the revisions they depend on. When a story or the code changes, whatever
depended on the old version becomes visibly stale, and the tool says exactly
what to revisit.

Nothing is demanded up front: a first change needs only its implementation
explained, and the rest of the Saga grows from there. The tool does not review
the code or generate a verdict; it helps the author prepare the material other
people review. Change Saga is experimental, and its format may change before
1.0.
