import assert from "node:assert/strict"
import {test} from "node:test"
import {
  frequencyAdvice,
  maximumSessionsInDays,
  minimumDaysForSessions,
} from "../src/features/schedule-patients/model/course-frequency.ts"

const rule = (minIntervalDays, defaultWeeklyCount) => ({
  minIntervalDays, defaultWeeklyCount,
})

test("zero days off still allows only one session per day", () => {
  assert.equal(maximumSessionsInDays(rule(1, 7), 1), 1)
  assert.equal(maximumSessionsInDays(rule(1, 7), 7), 7)
  assert.equal(minimumDaysForSessions(rule(1, 7), 10), 10)
  assert.match(frequencyAdvice(rule(1, 7), 2, 1), /одного сеанса в день/)
})

test("weekly cap applies to every rolling seven-day window", () => {
  assert.equal(maximumSessionsInDays(rule(1, 1), 7), 1)
  assert.equal(minimumDaysForSessions(rule(1, 1), 5), 29)
  assert.equal(maximumSessionsInDays(rule(1, 2), 7), 2)
  assert.equal(minimumDaysForSessions(rule(1, 2), 3), 8)
  assert.match(frequencyAdvice(rule(1, 1), 5, 7), /29 дн/)
})

test("full days off and weekly cap are both enforced", () => {
  assert.equal(maximumSessionsInDays(rule(3, 7), 7), 3)
  assert.equal(minimumDaysForSessions(rule(3, 7), 4), 10)
  assert.match(frequencyAdvice(rule(3, 7), 4, 7), /не менее 2 полных дней перерыва/)
  assert.equal(maximumSessionsInDays(rule(3, 1), 7), 1)
})

test("impossible course length is described rather than silently accepted", () => {
  assert.equal(minimumDaysForSessions(rule(2, 7), 365), null)
  assert.match(frequencyAdvice(rule(2, 7), 365, 365), /даже в пределах 365 дней/)
  assert.equal(frequencyAdvice(rule(1, 7), 5, 7), null)
})
