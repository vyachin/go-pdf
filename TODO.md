# TODO по результатам code review

Дата последней проверки: **24 ноября 2026 года**. Проверены CLI, HTML/CSS-парсеры, валидация, ресурсы, шрифты, текст, таблицы, пагинация, PDF, отладочный растр, тесты, документация и CI. Начальный снимок этого прохода — `a8dd72c`, финальный зафиксированный снимок — [2bdb8ff](https://github.com/vyachin/go-pdf/commit/2bdb8ff75940487715daa91cfa283d235586fb77). Локальный `main` в начале проверки совпадал с GitHub. **Добавлены 19 новых подтверждённых ошибок и три предложения по улучшению** — раздел «Дополнение: полное ревью текущего проекта» в конце файла.

Код в рамках этого ревью не исправлялся. Проверочные тесты создавались в отдельной временной копии; закрытые задачи удалены из списка после проверки исправлений. Во время проверки другие изменения продолжали поступать в `main`; результаты привязаны к указанным снимкам.

Приоритеты: **P1** — падение процесса, нарушение целостности документа или существенный риск для доступности; **P2** — неверное поведение поддерживаемой функции; **P3** — улучшение API, сопровождения или документации. Сначала закрыть задачи безопасности, затем корректность PDF и раскладки.

Ниже перечислены только открытые задачи; исправленные удалены после проверки.

## Улучшения для отдельной проработки

- [ ] **ENG-02 · P2: ввести бюджеты ресурсов и поддержать отмену во всех дорогих стадиях.**

  **Код:** [internal/engine/convert.go:21](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:21), [internal/engine/engine.go:62](/Users/vyachin/Projects/go-pdf/internal/engine/engine.go:62); парсеры, layout и загрузчики ресурсов.
  `ctx.Err()` проверяется только на входе. Предусмотреть отмену во время парсинга, обходов, пагинации и записи; ограничить размер HTML/CSS/шрифтов/изображений, глубину DOM, размер таблиц, число страниц и общий объём операций. Лимиты должны возвращать диагностируемую ошибку до опасной аллокации. Для CLI использовать `signal.NotifyContext` из стандартной библиотеки. Проверять отмену после старта работы, а не только уже отменённый context.

- [x] **DOC-01 · P2: согласовать обещанный библиотечный API с правилами `internal`.**

  **Код и документация:** [README.md](/Users/vyachin/Projects/go-pdf/README.md), раздел «Библиотека»; [internal/engine/engine.go](/Users/vyachin/Projects/go-pdf/internal/engine/engine.go).
  Пример предлагает внешнему пользователю импортировать `github.com/vyachin/go-pdf/internal/engine`, что Go запрещает за пределами родительского дерева. Выбрать публичный пакет с устойчивым API и публичными типами диагностик, либо явно описать CLI как единственный внешний интерфейс. Критерий: пример из README компилируется в отдельном модуле.

Штатные тесты исходного состояния `go test ./...` прошли. На обновлённом отдельном снимке штатные тесты и `go vet ./...` также прошли. Дополнительные проверки ревью размещены вне рабочего дерева: 20 проверок, из них 16 выявили ошибки; некоторые проверки относятся к одной задаче выше. Проверки не добавлялись в исходники и не изменяют корпус.

При реализации переносить каждый подтверждённый сценарий в постоянный regression-тест. Успешный прогон нынешних тестов не закрывает перечисленные дефекты: большинство из них текущими проверками не обнаруживается.

## Review addendum — 2026-11-24

This addendum records **23 newly confirmed defects (one P1 and 22 P2)** and two additional maintenance improvements. The earlier checklist and its completion state are preserved. Source code was not changed by this review.

The first review snapshot exactly matches commit [02dffa5](https://github.com/vyachin/go-pdf/commit/02dffa5cb7bfa4aa2c2b113ff25fbb30bad3e10e). All findings below were reproduced again against a separate snapshot of `705497a`, after the concurrent CSS and flow fixes. File links below refer to that later state. The six earlier tasks CSS-01, CSS-03, CSS-04, CSS-05, LAY-03 and LAY-04 now pass their targeted checks and are not duplicated here.

Only the findings that are still open are listed below. A successful existing test suite does not close them: each needs a regression test for the specific reproduction.

### Layout and pagination

- [ ] **LAY-14 · P2: apply vertical alignment inside table cells.**

  **Source:** [internal/layout/table.go:232](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:232), `paintCell`.
  **Reproduction:** `<table><tr><td style="vertical-align:bottom">BOTTOM</td><td>A<br>B<br>C</td></tr></table>` places `BOTTOM` on the first baseline, 28800 mp above `C`. `VerticalAlign` is parsed but the cell always paints at its top inset.
  **Fix / acceptance:** offset cell content using measured inner content height and available row height for top/middle/bottom alignment. Check unequal cell heights, padding and spans. If a value is intentionally unsupported, reject it instead of accepting it silently.

### CSS values and generated content

- [x] **CSS-07 · P2: apply specificity and importance to `a::after` content.**

  **Source:** [internal/css/cascade.go:60](/Users/vyachin/Projects/go-pdf/internal/css/cascade.go:60), `Compute`.
  **Reproduction:** `<style>a.hot::after{content:"CORRECT"}a::after{content:"WRONG"}</style><p><a class="hot">A</a></p>` renders `AWRONG`. Pseudo-element declarations are assigned during rule iteration before the normal cascade sorting, so later rules always win regardless of specificity or `!important`.
  **Fix / acceptance:** cascade generated content with the same importance, specificity and source-order rules used for other properties. Verify lower-specificity later rules, `!important`, and selector lists.

**Additional evidence for existing ENG-02:** cancellation one millisecond after starting a 1500-paragraph conversion left `ctx.Err()==context.Canceled`, but conversion returned `err=nil` and a 38433-byte PDF. Thread cancellation through parsing, resource loading, layout and writing rather than checking only on entry. For resource limits, also cover span dimensions (SEC-05), expanded GPOS ranges and decoded image pixel counts before their expensive loops/allocations.

- The existing suite passed on the original checkout and on the later `705497a` snapshot. In snapshots containing the additional probes, `go test ./... -skip '^TestReview'` runs the original suite.
- `go vet ./...` passed on both review snapshots.
- The initial snapshot had **36 review checks: 32 failed, four passed**. After rechecking `705497a`, **26 failed and ten passed**; six previously reported CSS/flow cases were fixed concurrently. These are scenario counts, not distinct-defect counts: CMYK and nested structure have more than one check, and cancellation updates existing ENG-02. The 26 failures correspond to the 23 new defects above plus those overlaps and the existing cancellation issue.
- Both review snapshots passed `go test -race ./... -skip 'TestReview|TestPerformanceBudget'`; no race was reported. The unrestricted race run failed the elapsed-time budget described in ENG-07. An early run against actively edited working files hit an intermediate syntax error; it was not treated as a stable project defect.
- The real Arial font was available, and `/tmp/gobin/pdfcpu` existed, so the existing font-dependent tests and their independent PDF validation ran. Structural validation alone does not verify color reproduction, glyph positions or visual completeness.
- Temporary probes and full machine-readable results are outside the working tree: [initial results](/private/tmp/go-pdf-review-uv291h3c/review-results.jsonl), [latest results](/private/tmp/go-pdf-review-current-dacig99u/review-results.jsonl). The four `review_probe_test.go` files in each snapshot contain the executable reproductions. These temporary paths are local review evidence; the reproductions and acceptance criteria above are the durable handoff.

## Дополнение: полное ревью текущего проекта — 24 ноября 2026 года

Ниже **19 новых ошибок уровня P2** и **три улучшения уровня P3**, без дублирования прежних задач. Все ошибки воспроизведены на отдельном снимке `2bdb8ff`; для проверок текста и геометрии использовался настоящий Arial. Приоритет внутри P2 выше у потери содержимого, ложного успеха валидации и проблем ресурсов. Ссылки на строки соответствуют проверенному снимку; при последующих изменениях ориентироваться также на имя функции.

### Блочная раскладка, списки и изображения

- [x] **LAY-19 · P2: реализовать размеры и атомарность `inline-block` либо отклонять его.**

  **Код:** [internal/layout/box.go:307](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:307), `isInlineDisplay`; [internal/layout/box.go:336](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:336), `buildElement`; `collectPieces`.
  **Ошибка:** `inline-block` строится как обычный `Inline`, затем рекурсивно разворачивается в слова. Его ширина, высота, фон и границы не формируют отдельного бокса; содержимое может переноситься как обычный текст.
  **Воспроизведение:** `<p><span style="display:inline-block;width:100pt;height:40pt;background-color:red">A</span>B</p>`. `B` начинается всего через **8004 mp** после `A`, вместо продвижения на **100000 mp**; отдельного красного бокса нет.
  **Исправление / готово, когда:** измерять и размещать inline-block как единый элемент строки со своими размерами и декорациями. Проверить соседний текст, высоту строки и перенос целого бокса. Если эта возможность пока вне профиля, валидатор должен её отклонять.

### Пробелы, наследование и генерируемое содержимое CSS

- [x] **TEXT-01 · P2: схлопывать пробелы на границах inline-элементов.**

  **Код:** [internal/layout/lines.go:198](/Users/vyachin/Projects/go-pdf/internal/layout/lines.go:198), обработка `spacePiece`; `text.Split`.
  **Ошибка:** пробелы схлопываются внутри отдельного текстового узла, но состояние не переносится между узлами. Последовательные `SpaceItem` добавляются в строку и каждый увеличивает её ширину; концевые пробелы также не удаляются перед выравниванием.
  **Воспроизведение:** `<p>A <span> B</span> C</p>` даёт **`A  B C`** с двумя пробелами после `A`, вместо одного. Это меняет переносы, центрирование и ширину строки.
  **Исправление / готово, когда:** схлопывать пробелы на уровне всего inline-потока и удалять концевые схлопываемые пробелы перед расчётом строки. Проверить границы нескольких вложенных span, разрывы строк, NBSP и сохранение пробелов при `pre`.

### Предложения по улучшению

- [ ] **ARCH-01 · P3: объединить расчёт геометрии и правила потока для обычных блоков и ячеек.**

  **Код:** [internal/layout/paginate.go](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go), `measure`/`placeBlock`; [internal/layout/table.go](/Users/vyachin/Projects/go-pdf/internal/layout/table.go), `paintStaticBlock`/`paintStaticTable`.
  Разные реализации уже расходятся по margins (LAY-07), изображениям (LAY-17), ширине и декорациям. Выделить общий расчёт used-размеров и статическую раскладку содержимого; пагинация должна фрагментировать рассчитанные боксы, а рисование — использовать их геометрию. Делать постепенно через сравнение координат одинакового блока внутри и вне ячейки, без полной переписи движка.

- [ ] **ENG-09 · P3: добавить независимое визуальное и текстовое сравнение в CI.**

  **Код:** [.github/workflows/ci.yml](/Users/vyachin/Projects/go-pdf/.github/workflows/ci.yml), [internal/engine/corpus_test.go](/Users/vyachin/Projects/go-pdf/internal/engine/corpus_test.go), [internal/pdfdebug/extract.go](/Users/vyachin/Projects/go-pdf/internal/pdfdebug/extract.go).
  Обязательный `pdfcpu` уже полезен, но проверяет структуру PDF; собственный растр получает операции layout до PDF-сериализации и не доказывает правильность конечного файла. Добавить небольшой фиксированный корпус конечных PDF, растеризацию независимым движком и независимое извлечение текста. Закрепить версии шрифтов/инструментов; включить alpha, CMYK/rejection, несколько шрифтов, Unicode-алиасы и номера страниц. Проверять координаты/области с допуском, сохраняя сравнения текста и графа объектов; растеризаторы нужны только в CI, а не в runtime библиотеки.

- [x] **DOC-02 · P3: исправить порядок аргументов в первом CLI-примере README.**

  **Код:** [README.md:10](/Users/vyachin/Projects/go-pdf/README.md:10), [cmd/pdfgen/main.go:91](/Users/vyachin/Projects/go-pdf/cmd/pdfgen/main.go:91), `flag.FlagSet.Parse`.
  `pdfgen convert invoice.html -o invoice.pdf` из вступления возвращает **код 2** и `exactly one input file is required`: стандартный Go flag прекращает разбор флагов после первого позиционного аргумента. Исправить на `pdfgen convert -o invoice.pdf invoice.html`, согласовать остальные примеры и явно описать порядок. Поддержка флагов после имени файла — отдельное решение API, для исправления документации она не требуется.

- `go vet ./...` прошёл на исходном и финальном снимках.
- Первый обычный `go test ./...` на `a8dd72c` упал только на **TestPerformanceBudget**: одна страница заняла **107,32525 мс** при лимите **50 мс**. Это дополняет ENG-07: нестабильность проявилась даже без `-race` при параллельной нагрузке. Функциональные пакеты прошли; падение нельзя скрывать формулировкой «все тесты зелёные».
- На финальном снимке `2bdb8ff` прошёл полный функциональный прогон с гонками: `go test -race ./... -skip '^TestReview|^TestPerformanceBudget$'`. Гонок не обнаружено. `TestReview` — только временные проверки этого ревью, отсутствующие в рабочем проекте; тест производительности исключён явно по ENG-07.
- Корпуса `internal/engine` и `internal/mpdfcompat` отдельно прошли с обязательным независимым валидатором: `PDF_REQUIRE_VALIDATOR=1 PDFCPU=/tmp/gobin/pdfcpu go test ./internal/engine ./internal/mpdfcompat -run 'TestConvertProducesValidPDF|TestCorpus' -count=1`. На финальном снимке корпус также прошёл в составе race-прогона с доступным pdfcpu. Наличие этих проходов не опровергает ошибки внешнего вида и текстового слоя выше.
- `go test ./internal/mpdfcompat -short -count=1` прошёл. `openspec validate --specs` принял все **5** основных спецификаций; предупреждения касались формулировок нормативных требований на русском.
- Проверки путей, геометрии, alpha, структуры ToUnicode, валидации и отмены запускались в отдельной временной копии. Временные негативные проверки намеренно падают, фиксируя расхождение с ожидаемым поведением; они не добавлены в штатный набор проекта. При исправлении задачи перенести её сценарий в постоянный regression-тест.
- **ENG-02 повторно подтверждён:** отмена через 1 мс оставляет `ctx.Err()==context.Canceled`, но конвертация возвращает `err=nil` и **230953 байта PDF**. Также `ParseLengthString("1e100pt")` принимает число без ошибки и на проверенной платформе превращает его в `2147483647` mp. Проверять конечность/диапазон до приведения к `int32`, затем контролировать арифметику размеров и лимиты декодирования/растеризации. Опасные OOM-сценарии намеренно не запускались.
- Исправления прежних HTML-01, HTML-02, PDF-04, PDF-05, PDF-06, FONT-01, FONT-02, FONT-03, CSS-06 и CSS-08 поступили отдельными изменениями в `main` во время этого прохода. Они учтены в финальном снимке и не включены в 19 новых ошибок. Остальные ранее открытые задачи сохраняют свои идентификаторы.
- Не проводилась проверка соответствия PDF/A, PDF/UA или полного браузерного CSS: проект этого не обещает. Новые ошибки относятся к принятому профилю, заявленному API или собственным гарантиям проекта.
