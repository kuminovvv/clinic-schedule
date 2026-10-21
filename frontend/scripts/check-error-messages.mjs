// Plain-language mapping regression. Node 22+ supports TS type stripping.
import assert from "node:assert/strict"
import {explainApiError} from "../src/shared/api/error-messages.ts"

const cases = [
  [{status:409, reason:"procedure is used in one or more patient plans", path:"/v1/procedures/1",method:"DELETE"}, "Процедуру пока нельзя удалить"],
  [{status:409, reason:"patient plan changed; reload before saving",path:"/v1/day-plans",method:"PUT"}, "Данные изменились в другом окне"],
  [{status:409, reason:"procedure cannot repeat before its minimum interval",path:"/v1/appointments/1",method:"PUT"}, "Эту процедуру пока нельзя повторить"],
  [{status:409, reason:"appointment time is already occupied",path:"/v1/appointments/1",method:"PUT"}, "Выбранное время занято"],
  [{status:422, reason:"appointment is outside patient treatment period",path:"/v1/appointments/1",method:"PUT"}, "Дата вне срока пребывания"],
  [{status:422, reason:"some procedures do not fit",path:"/v1/schedules/generate",method:"POST"}, "Не все процедуры помещаются в курс"],
  [{status:400, reason:"invalid procedure",path:"/v1/procedures",method:"PUT"}, "Проверьте данные процедуры"],
  [{status:503, reason:"database unavailable",path:"/v1/appointments",method:"GET"}, "Программа временно недоступна"],
  [{status:200, reason:"invalid response body",path:"/v1/appointments",method:"GET"}, "Не удалось прочитать полученные данные"],
  [{path:"/v1/appointments",method:"GET"}, "Не удалось связаться с программой"],
]

for (const [request, title] of cases) {
  const notice = explainApiError(request)
  assert.equal(notice.title, title)
  assert.ok(notice.description.length > 12)
  assert.ok(!notice.description.includes("HTTP") && !notice.description.includes("backend"))
}
console.log(`OK: ${cases.length} API errors have clear Russian-language messages`)
