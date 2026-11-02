## The problem {#the-problem}

Change Saga's review deck explains a change after it is built. That is when a reviewer can check the work against the code, and it is also when a disagreement about the approach is most expensive: by then someone, often an agent, has already written it. Teams still talk about the approach beforehand, but in chat, in a pull request description, or in a document that nothing links to the work that follows.

## What a Plan is {#what-a-plan-is}

**A Plan is what you will do; a Review is what you did.** A Plan is a slide deck plus an essay that describes how a team will build a feature, written before the code exists. It reads like a review deck and is discussed like one, slide by slide, but it explains intent instead of a diff, so it owes no code coverage and claims none.

## Who it serves {#who-it-serves}

- An **architect** can set out the approach while changing it is still a conversation.
- A **developer** can question that approach on the slide that shows it, and later, reviewing the change, can see where the work departed from what was agreed.
- A **product manager** can check that the approach serves the stories it says it delivers, and can tell a live Plan from a finished or abandoned one.

## How it relates to what exists {#how-it-relates}

A Plan does not replace any of these, and none of them requires a Plan:

| Record | What it answers | When |
| --- | --- | --- |
| Story and criteria | What should a person be able to do? | Before design |
| **Plan** (planned) | How will we build it, and do we agree? | Before code |
| Work plan (`change-saga plan`) | Who builds which part, in what order? | While building |
| Review | What did this pull request do, and is every changed line explained? | At the pull request |
| Implementation deck | How does it work now? | After it ships |

This feature augments [Rich visual review](urn:change-saga:change-saga:feature:visual-review) rather than standing alone. A Plan is a new kind of review: it uses the same deck, the same per-slide decisions and comments, and the same reading surface, and a reader looking for how a team reviews should find both in one place. When Plans are built, `feature deliver` will move these stories there. Whether Plans deserve their own durable feature instead is listed below.

## What is true today {#what-is-true-today}

Nothing here is built. There is no Plan record, command, or page. These stories are proposed: they record the intent as stated and have not been accepted. The stories' criteria describe outcomes, not a chosen design.

The design itself is the deck under this feature, [How we would build Plans](urn:change-saga:change-saga:deck:plan-for-plans), which is also the trial of the idea: a Plan for Plans, written using only what exists today. It sits in a UX deck because that is the only feature deck that does not claim to explain code it has never seen, and it is not a UX flow. It has no approvals and no comments, because the Saga carries none outside reviews. Those gaps are the case for this feature.

## What the trial taught us {#what-the-trial-taught-us}

Writing the Plan for Plans with today's tools found these limits, each of which shaped a story:

- **There is no container shaped like a Plan.** A UX deck reads as user flows in the navigation. An implementation deck would sit under Technical, which promises a description of current code. This is why a Plan must say plainly what it is.
- **`apply-slide` asks UX decks for code.** The specification says a UX deck's slides are not asked for code, but complete-slide publishing refuses an Item without evidence, so a code-free deck loses diagram sources, element notes, and `diagram edit`.
- **Nowhere to agree.** The deck can be read but not approved or commented on, so its review has to happen somewhere nothing links to.
- **A slide cannot link to its story honestly.** The relation that connects a UX Item to a criterion also earns that criterion its UX coverage, which a plan has not done. The Plan for Plans therefore links to stories only from its prose.
- **Nothing retires it.** `feature deliver` would move this deck into Rich visual review with the stories, where it would read as a description of current behavior until someone removes it by hand.

A Review can in fact hold a Plan today: in a scratch copy, `review create` on a branch with no code gave working per-slide approvals and comments, reporting 0 of 0 lines covered. That is the strongest evidence for treating a Plan as a kind of review. It stops working the moment code lands on that branch, because the same review then owes coverage of every changed line.

## Open questions {#open-questions}

These are not decided. The deck recommends an answer where it has one.

1. **Own feature, or part of Rich visual review?** The augments link is a judgement, and `feature deliver` takes one recipient. Stories about decisions could as easily live under Review decisions and attribution.
2. **What is it called?** `change-saga plan` already means the work plan, and each feature's `plan.json` already means Roadmap status. "Plan" would be the third meaning.
3. **One Plan to how many Reviews?** A Plan delivered in phases needs several; an integration branch needs one.
4. **What counts as agreed?** The format records and teams decide, so no Plan is ever declared approved. Should a team be able to require approvals before work starts, the way repository policy can gate a merge?
5. **Keep or delete a closed Plan?** A shipped Plan is half of the comparison with its Review, which argues for keeping it frozen. An abandoned one has less claim to stay, given that the Saga is current state and Git keeps the history. Yet why a team decided not to build something is what Git is worst at surfacing.
6. **Who closes a Plan?** Closing could stay explicit, or `status` could suggest it when every linked story is built.
7. **Should a Plan's slides cite existing code?** Pinning the code a Plan builds on would make it verifiable, and would make unrelated pull requests repin it.
8. **Is an agreed Plan an agent's brief?** The stories guidance already describes a technical plan as the specification later AI work references. Whether an agent should be handed a Plan directly, and held to it, is untested.
