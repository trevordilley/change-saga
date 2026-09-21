# Code evidence technical model {#code-evidence-technical-model}

This chapter is the technical contract beneath the user-facing evidence,
currency, and repair experience. The experience owns observable states and safe
outcomes; this chapter owns durable evidence identity, source-side rules, and
the boundary around replaceable resolution algorithms.

A code reference records source, never a diff. A diff is a view used to explain
why a reference is current, remapped, or stale at another source revision. The
implementation deck may illustrate the current resolver, but neither its
package structure nor one line-matching algorithm is part of the UX contract.
