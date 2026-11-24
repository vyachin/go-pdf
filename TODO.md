# TODO по результатам code review

Дата последней проверки: **24 ноября 2026 года**. Проверены CLI, HTML/CSS-парсеры, валидация, ресурсы, шрифты, текст, таблицы, пагинация, PDF, отладочный растр, тесты, документация и CI. Начальный снимок этого прохода — `a8dd72c`, финальный зафиксированный снимок — [2bdb8ff](https://github.com/vyachin/go-pdf/commit/2bdb8ff75940487715daa91cfa283d235586fb77). Локальный `main` в начале проверки совпадал с GitHub. **Добавлены 19 новых подтверждённых ошибок и три предложения по улучшению** — раздел «Дополнение: полное ревью текущего проекта» в конце файла.

Код в рамках этого ревью не исправлялся. Проверочные тесты создавались в отдельной временной копии; прежние задачи и отметки об исправлениях сохранены. Описания закрытых задач ниже относятся к состоянию на момент их обнаружения, а не означают наличие ошибки в текущем коде. Во время проверки другие изменения продолжали поступать в `main`; результаты привязаны к указанным снимкам.

Приоритеты: **P1** — падение процесса, нарушение целостности документа или существенный риск для доступности; **P2** — неверное поведение поддерживаемой функции; **P3** — улучшение API, сопровождения или документации. Сначала закрыть задачи безопасности, затем корректность PDF и раскладки.

Ближайшие работы: **ENG-02** — отмена и бюджеты ресурсов; **ASSET-01 + ENG-08** — корректные пути и освобождение дескрипторов; **LAY-17 + PDF-07** — потеря изображения и подмена символов в текстовом слое; **VAL-01 + VAL-02** — достоверность проверки шаблона. Затем — остальные ошибки геометрии и CSS. Ранее предложенные SEC-04, CLEAN-02, ENG-05 и базовый CI уже реализованы.

## Безопасность и устойчивость

- [x] **SEC-01 · P1: исключить панику при разборе повреждённой таблицы GPOS.**

  **Код:** [internal/font/gpos.go:213](/Users/vyachin/Projects/go-pdf/internal/font/gpos.go:213), `parseClassDef`.
  **Ошибка:** перед чтением заголовка проверяются четыре байта, но для формата 1 затем читается диапазон `off+4:off+6`. Усечённые данные вызывают `slice bounds out of range`, а не диагностируемую ошибку. Дефект достижим через `font.Parse` и шрифт из `@font-face`.
  **Воспроизведение:** `parseClassDef([]byte{0, 1, 0, 0}, 0)`; также проверен полный TrueType с усечённой GPOS длиной 48 байт и ограниченной capacity входного среза.
  **Исправление:** проверять длину заголовка отдельно для каждого формата до чтения; проверить аналогичные границы во всём бинарном парсере.
  **Готово, когда:** повреждённый шрифт возвращает ошибку или безопасно игнорирует необязательную повреждённую таблицу; нет паники. Добавлен regression-тест через `font.Parse` и seed для fuzz-тестирования.

- [x] **SEC-02 · P2: обеспечить границу `BaseDir` при чтении CSS через символические ссылки.**

  **Код:** [internal/profile/styles.go:124](/Users/vyachin/Projects/go-pdf/internal/profile/styles.go:124), `resolveStylesheetPath` и `loadStylesheet`.
  **Ошибка:** `filepath.Clean` и `filepath.Rel` проверяют только лексический путь. Символическая ссылка внутри базового каталога позволяет прочитать CSS снаружи, хотя API обещает жёсткую границу.
  **Воспроизведение:** создать `base/link.css -> ../outside.css`; при `BaseDir=base` документ `<link rel="stylesheet" href="link.css">` принимается и читает внешний файл. Подтверждено тестом.
  **Исправление:** использовать ограниченное корнем открытие файла; проверить `os.OpenRoot` / `os.Root` из стандартной библиотеки Go. Одного `EvalSymlinks` перед обычным `ReadFile` недостаточно для защиты от замены ссылки между проверкой и открытием.
  **Готово, когда:** прямой выход через `..`, абсолютный путь наружу и выход через symlink отклоняются; допустимые ссылки внутри корня работают. Политика доступа к изображениям и шрифтам отдельно определена и задокументирована.

- [x] **SEC-03 · P1: прекращать пагинацию без прогресса и не возвращать успешный пустой документ.**

  **Код:** [internal/layout/paginate.go:91](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:91), `paginate`, `placeBlock`, `placeText`.
  **Ошибка:** контейнер помечает страницу непустой до размещения содержимого. Строка выше доступной области постоянно переносится на следующую страницу. Лимит в 5000 итераций завершает цикл предупреждением, сохраняя пустые страницы и теряя текст. Это также ненужный расход памяти и CPU на малом входе.
  **Воспроизведение:** `<style>@page{size:100pt 100pt;margin:10pt}p{font-size:10pt;line-height:100pt;margin:0}</style><p>IMPORTANT</p>`. Получено **5001 страница, 0 текстовых операций**, только `warning[layout/loop]`.
  **Исправление:** учитывать фактический прогресс и фактическую заполненность страницы; назначить одно место, управляющее переходом на новую страницу. Неразмещаемое содержимое должно давать явную ошибку либо согласованное поведение overflow с сохранением содержимого.
  **Готово, когда:** этот пример завершается за ограниченное число шагов; незавершённая раскладка не выдаётся как успешный PDF. Проверены высокая строка, изображение и строка таблицы.

## Корректность PDF

- [x] **PDF-01 · P1: исправить граф объектов встроенного шрифта.**

  **Код:** [internal/pdf/writer.go:242](/Users/vyachin/Projects/go-pdf/internal/pdf/writer.go:242), `Doc.Bytes`.
  **Ошибка:** на шрифт выделяется четыре объекта, хотя нужны пять: Type0, CIDFontType2, FontDescriptor, FontFile2, ToUnicode. `w.set(descID, ...)` сначала записывает CIDFontType2, затем перезаписывает его FontDescriptor. В результате `/DescendantFonts` ссылается на дескриптор вместо CID-шрифта. Дефект затрагивает любой документ с текстом.
  **Воспроизведение:** конвертировать `<p>Hello</p>` и разрешить ссылку `/DescendantFonts`: объект имеет `/Type /FontDescriptor`, а не `/Subtype /CIDFontType2`. Подтверждено разбором результата.
  **Исправление:** выделять отдельные идентификаторы всем пяти объектам; проверить цепочку Type0 → CIDFontType2 → FontDescriptor → FontFile2 и отдельную ссылку ToUnicode.
  **Готово, когда:** независимый валидатор принимает документ, текст виден в независимом PDF-рендерере, русский текст корректно извлекается. Наличие строк `/ToUnicode` и `/Identity-H` само по себе не считается проверкой валидности.

- [x] **PDF-02 · P2: сделать порядок ресурсов изображений детерминированным.**

  **Код:** [internal/pdf/writer.go:273](/Users/vyachin/Projects/go-pdf/internal/pdf/writer.go:273), сборка `/XObject`.
  **Ошибка:** ресурсы сериализуются обходом `map`, порядок которого не гарантирован. Меняются байты PDF и вычисляемый `/ID` при одинаковом входе.
  **Воспроизведение:** 30 вызовов `Bytes()` для одного документа с тремя изображениями дали **3 варианта PDF**.
  **Исправление:** обходить уже упорядоченный срез `images`, либо сортировать ключи перед сериализацией.
  **Готово, когда:** многократная сериализация документов с несколькими изображениями и страницами байтово идентична; проверка есть в тестах.

- [x] **PDF-03 · P2: передавать кернинг из раскладки в PDF.**

  **Код:** [internal/engine/convert.go](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go), `toPDFOps`; [internal/pdf/writer.go:377](/Users/vyachin/Projects/go-pdf/internal/pdf/writer.go:377), `contentStream`.
  **Ошибка:** рассчитанный `Glyph.Advance` теряется при преобразовании в `pdf.Glyph`. PDF использует обычный `Tj` с номинальными ширинами, хотя переносы и координаты следующих слов рассчитаны с кернингом.
  **Воспроизведение:** Arial, 12 pt, `AVATAR`: ширина layout **44448 mp**, сумма номинальных advances **48012 mp**; в PDF нет корректировок расстояний. Поэтому PDF отличается от собственного растра и может нарушать рассчитанные интервалы.
  **Исправление:** сохранить advances и выразить позиционирование через `TJ` либо эквивалентные корректировки.
  **Готово, когда:** положения глифов в PDF совпадают с layout для латиницы и кириллицы с ненулевым кернингом; проверен независимый рендер.

## Таблицы, CSS и раскладка

- [x] **LAY-01 · P2: учитывать занятые `rowspan` колонки в следующих строках.**

  **Код:** [internal/layout/table.go:163](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:163), `paintRow`, `rowHeight`, `computeColumns`.
  **Ошибка:** каждая строка начинает размещение с колонки 0, без учёта ячеек, продолжающихся из предыдущих строк. Увеличения высоты объединённой ячейки недостаточно.
  **Воспроизведение:** `<table><tr><td rowspan="2">A</td><td>B</td></tr><tr><td>C</td></tr></table>`. Координаты X: `A=59528`, `B=300473`, `C=59528`; C попадает под A вместо колонки B.
  **Исправление:** построить сетку занятости колонок и использовать её при измерении и отрисовке; согласовать перенос через страницу.
  **Готово, когда:** C выравнивается с B; есть тесты сочетаний `rowspan`/`colspan` и переноса таблицы.

- [x] **LAY-02 · P2: не терять вторую и последующие строки `thead` на первой странице.**

  **Код:** [internal/layout/table.go:142](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:142), `placeTable`.
  **Ошибка:** условие пропуска строк считает их уже повторённой шапкой и на исходной странице, где повтор ещё не выполнялся.
  **Воспроизведение:** таблица с `HEADER1`, `HEADER2` в двух строках `thead` и `BODY` в `tbody` выводит только `HEADER1BODY`.
  **Исправление:** отделить исходное размещение шапки от её повтора на продолжении таблицы.
  **Готово, когда:** обе строки присутствуют ровно один раз на первой странице и повторяются ровно один раз на каждой странице продолжения.

- [x] **CSS-01 · P2: применять вложенные правила `@media print` и `@media all`.**

  **Код:** [internal/css/cascade.go:32](/Users/vyachin/Projects/go-pdf/internal/css/cascade.go:32); также сборка `@page` и `@font-face`.
  **Ошибка:** валидатор принимает MediaRule, но потребители листов обходят только верхний уровень. Правила внутри разрешённого `@media` теряются.
  **Воспроизведение:** `<style>@media print{p{display:none}}</style><p>HIDDEN</p>` всё равно выводит HIDDEN.
  **Исправление:** централизованно обходить применимые вложенные правила с сохранением порядка каскада; использовать одинаковую модель в каскаде, настройках страницы и реестре шрифтов.
  **Готово, когда:** проверены `print`, `all`, вложенность, порядок правил, `@page` и `@font-face` внутри применимого блока.

- [x] **LAY-03 · P2: размещать дочерние блоки относительно content box родителя.**

  **Код:** [internal/layout/paginate.go:126](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:126), `placeBlock` и `placeChild`.
  **Ошибка:** X каждого блока берётся из левого поля страницы. Горизонтальные padding и border родительских блоков не передаются детям; горизонтальные margin также не участвуют в размещении.
  **Воспроизведение:** `<div style="padding:0 0 0 20pt"><p>PADDED</p></div>`: текст начинается с **56693 mp** вместо **76693 mp**.
  **Исправление:** передавать дочернему размещению начало и ширину content box родителя; учитывать поддерживаемые горизонтальные отступы.
  **Готово, когда:** вложенность, padding, border, фиксированная ширина и margin дают ожидаемые X и ширины, включая продолжение на другой странице.

- [x] **LAY-04 · P2: включать нижние padding и border в вертикальное продвижение потока.**

  **Код:** [internal/layout/paint.go:12](/Users/vyachin/Projects/go-pdf/internal/layout/paint.go:12), `finishFragment`; [internal/layout/paginate.go:243](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:243), `afterBlock`.
  **Ошибка:** нижние padding и border добавляются к нарисованной геометрии, но не увеличивают `flow.y`. Следующий блок начинается внутри нижней части предыдущего.
  **Воспроизведение:** `<div style="padding:10pt">A</div><p style="margin:0">B</p>` даёт расстояние между базовыми линиями **14400 mp** вместо хотя бы **24400 mp**.
  **Исправление:** завершение блока должно продвигать поток на нижние padding и border; отдельно согласовать поведение фрагментов на границе страниц.
  **Готово, когда:** следующий блок не перекрывает padding/border предыдущего; тесты проверяют координаты, а не только размеры прямоугольников.

- [x] **CSS-02 · P2: восстановить наследование от `html`, групп строк и `tr`; сохранять атрибуты `html`.**

  **Код:** [internal/layout/box.go:231](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:231), `Build`; [internal/layout/box.go:401](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:401), `buildTable`; [internal/html/parse.go:117](/Users/vyachin/Projects/go-pdf/internal/html/parse.go:117), `ensureHTML`.
  **Ошибка:** body вычисляется с `parent=nil`, а стиль ячейки — непосредственно от table. Промежуточные предки пропускаются. Кроме того, при создании html теряются его атрибуты.
  **Воспроизведение:** `html{font-size:30pt}` оставляет текст в body размером **12 pt**; `tr{color:red}` не окрашивает текст td; `<html style="color:red">` тоже не действует.
  **Исправление:** вычислять стили по действительной цепочке DOM и сохранять атрибуты корневого элемента.
  **Готово, когда:** наследование шрифта, цвета и выравнивания работает через html/body и table/thead/tbody/tr/td; классы, id и inline style на html доступны каскаду.

- [x] **CSS-03 · P2: обрабатывать переводы строк и пробелы в `white-space:pre` до шейпинга.**

  **Код:** [internal/text/normalize.go:111](/Users/vyachin/Projects/go-pdf/internal/text/normalize.go:111), `Split`; [internal/layout/lines.go](/Users/vyachin/Projects/go-pdf/internal/layout/lines.go), `collectPieces`.
  **Ошибка:** весь preformatted-текст становится одним словом, включая управляющие символы. Перевод строки передаётся в поиск глифа, и валидный текст отклоняется.
  **Воспроизведение:** `<p style="white-space:pre">A\nB</p>` с настоящим LF даёт `text/glyph`, U+000A в Arial.
  **Исправление:** представить переводы строк и сохраняемые пробелы отдельными элементами потока; определить поведение табуляции.
  **Готово, когда:** строки отображаются раздельно, последовательности пробелов сохраняются, перенос строки не требует глифа. Проверены LF и CRLF.

- [x] **CSS-04 · P2: применять `text-indent` к первой строке.**

  **Код:** [internal/layout/box.go:302](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:302), `anonymousStyle`; [internal/layout/paint.go:74](/Users/vyachin/Projects/go-pdf/internal/layout/paint.go:74), `paintLineAt`.
  **Ошибка:** анонимный блок сбрасывает отступ; отрисовка также не добавляет `line.Indent` к X, хотя расчёт ширины учитывает отступ. Свойство принимается, но текст не сдвигается.
  **Воспроизведение:** `<p style="text-indent:20pt;margin:0">INDENT</p>` начинает текст с **56693 mp** вместо **76693 mp**.
  **Исправление:** сохранить нужное наследование и согласовать X, доступную ширину и выравнивание первой строки.
  **Готово, когда:** первая строка сдвинута, последующие — нет; работают длины и проценты, выравнивание и перенос абзаца на другую страницу.

- [x] **CSS-05 · P2: сохранять множитель безразмерного `line-height` при наследовании.**

  **Код:** [internal/css/style.go:68](/Users/vyachin/Projects/go-pdf/internal/css/style.go:68), `NewStyle`, `resolveEm`, `LineHeightMP`.
  **Ошибка:** безразмерный множитель преобразуется в абсолютную длину на родителе, затем наследуется как фиксированный размер. При изменении font-size у ребёнка ожидаемый интервал теряется.
  **Воспроизведение:** `<div style="font-size:10pt;line-height:2"><p style="font-size:20pt;margin:0">A<br>B</p></div>`: расстояние **22631 mp**, а должно быть **40000 mp**.
  **Исправление:** отличать безразмерный множитель от `em`, процентов и абсолютной длины; вычислять его used value по размеру шрифта текущего элемента.
  **Готово, когда:** `line-height:2` корректно наследуется при изменении размера шрифта, а заданный абсолютной длиной интервал сохраняет свою семантику.

## Улучшения для отдельной проработки

- [x] **ENG-01 · P2: сделать независимую проверку PDF обязательной в CI.**

  **Код:** [internal/engine/convert_test.go](/Users/vyachin/Projects/go-pdf/internal/engine/convert_test.go), `validateWithPDFCPU`; [internal/mpdfcompat/corpus_test.go](/Users/vyachin/Projects/go-pdf/internal/mpdfcompat/corpus_test.go), `validatePDF`.
  Сейчас отсутствие внешнего валидатора позволяет пройти всем тестам даже при PDF-01. Собственный `pdfdebug` извлекает текст, но не проверяет весь граф объектов и не доказывает видимость текста. В CI закрепить валидатор и независимый рендерер; проверять несколько страниц, шрифты и изображения. В локальном режиме явно сообщать о пропущенной независимой проверке. Для поиска инструмента в PATH использовать стандартный `exec.LookPath`, а не только жёстко заданный `/tmp/gobin/pdfcpu`.
  **CI:** remote `origin` настроен, но каталога `.github/workflows` нет. Добавить workflow с `go vet ./...`, `go test ./...` и `openspec validate --specs`, а независимую проверку PDF сделать отдельным обязательным шагом.

- [ ] **ENG-02 · P2: ввести бюджеты ресурсов и поддержать отмену во всех дорогих стадиях.**

  **Код:** [internal/engine/convert.go:21](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:21), [internal/engine/engine.go:62](/Users/vyachin/Projects/go-pdf/internal/engine/engine.go:62); парсеры, layout и загрузчики ресурсов.
  `ctx.Err()` проверяется только на входе. Предусмотреть отмену во время парсинга, обходов, пагинации и записи; ограничить размер HTML/CSS/шрифтов/изображений, глубину DOM, размер таблиц, число страниц и общий объём операций. Лимиты должны возвращать диагностируемую ошибку до опасной аллокации. Для CLI использовать `signal.NotifyContext` из стандартной библиотеки. Проверять отмену после старта работы, а не только уже отменённый context.

- [x] **ENG-03 · P3: убрать повторное чтение CSS и дедуплицировать ресурсы на документ.**

  **Код:** [internal/engine/convert.go:29](/Users/vyachin/Projects/go-pdf/internal/engine/convert.go:29), [internal/profile/profile.go](/Users/vyachin/Projects/go-pdf/internal/profile/profile.go), [internal/engine/images.go](/Users/vyachin/Projects/go-pdf/internal/engine/images.go).
  Валидация и конвертация отдельно собирают и читают листы CSS. Загружать их один раз, валидировать и использовать один неизменяемый результат; ошибки чтения не должны обходить валидацию при повторной загрузке. Кэшировать изображения между проходами layout и сохранять общую идентичность PDF-ресурса. Критерии: один снимок входных ресурсов на конвертацию, повторное изображение не встраивается заново для каждого вхождения.

- [ ] **ENG-04 · P3: заменить самописные служебные операции стандартной библиотекой.**

  [internal/profile/styles.go](/Users/vyachin/Projects/go-pdf/internal/profile/styles.go): заменить `asLinkError` на `errors.As`; границу каталога реализовать через `os.OpenRoot` / `os.Root` с учётом SEC-02.
  [cmd/pdfgen/main.go](/Users/vyachin/Projects/go-pdf/cmd/pdfgen/main.go), `writeFileAtomic`: использовать `filepath.Dir` и `filepath.Base` вместо ручного поиска `/`; сохранить `os.CreateTemp` и `os.Rename` для записи в том же каталоге.
  [internal/engine/images.go](/Users/vyachin/Projects/go-pdf/internal/engine/images.go): рассмотреть `image.DecodeConfig`, возвращающий формат, вместо определения формата только по расширению и дублирования веток PNG/JPEG. Сохранить закрытый список разрешённых форматов и добавить лимит пикселей до декодирования.

- [ ] **DOC-01 · P2: согласовать обещанный библиотечный API с правилами `internal`.**

  **Код и документация:** [README.md](/Users/vyachin/Projects/go-pdf/README.md), раздел «Библиотека»; [internal/engine/engine.go](/Users/vyachin/Projects/go-pdf/internal/engine/engine.go).
  Пример предлагает внешнему пользователю импортировать `github.com/vyachin/go-pdf/internal/engine`, что Go запрещает за пределами родительского дерева. Выбрать публичный пакет с устойчивым API и публичными типами диагностик, либо явно описать CLI как единственный внешний интерфейс. Критерий: пример из README компилируется в отдельном модуле.

- [ ] **CLEAN-01 · P3: удалить одноразовый `cmd/mpdfimport` после завершённого импорта.**

  Запрос пользователя: импортер больше не нужен, поскольку корпус уже сохранён. Проверено: тесты читают HTML непосредственно из [testdata/mpdf/cases.json](/Users/vyachin/Projects/go-pdf/testdata/mpdf/cases.json); для запуска Go-тестов PHP-репозиторий и импортер не требуются.
  При реализации удалить команду и её собственные тесты, обновить сообщение об отсутствующем корпусе и комментарий в [internal/mpdfcompat/corpus_test.go](/Users/vyachin/Projects/go-pdf/internal/mpdfcompat/corpus_test.go), убрать инструкцию запуска удалённой команды из [docs/mpdf-corpus.md](/Users/vyachin/Projects/go-pdf/docs/mpdf-corpus.md). Сохранить cases.json, goldens и сведения о происхождении. Проверка: `go test ./...` и отсутствие действующих ссылок на удалённую команду.

- [x] **SEC-04 · P3: `ReadWithin` смешивает «вне корня» с прочими ошибками открытия.**

  **Код:** [internal/profile/styles.go](/Users/vyachin/Projects/go-pdf/internal/profile/styles.go), `ReadWithin`.
  Сейчас любая ошибка `os.Root.Open`, кроме отсутствия файла (например, нет прав чтения), возвращается как `ErrOutsideRoot`, и пользователь видит «escapes the base directory» вместо реальной причины. Поведение безопасное (fail-closed), но диагностика вводит в заблуждение. **Исправление:** отличать выход за корень от прочих ошибок (по `*os.PathError`, до появления сентинела в стандартной библиотеке), остальные возвращать как есть.
  **Связь:** выполнять вместе с CLEAN-02 — после переноса в `internal/fsx` строгая обработка ошибок живёт в одном месте и не смешивается с правилами документа.
  **Готово, когда:** файл без прав чтения даёт ошибку доступа, а не «вне корня»; есть тест на оба случая.

- [x] **ENG-05 · P3: не разрешать корень повторно для каждого актива.**

  **Код:** [internal/profile/styles.go](/Users/vyachin/Projects/go-pdf/internal/profile/styles.go), `ReadWithin`.
  `EvalSymlinks(rootDir)` и `os.OpenRoot` вызываются на каждый файл, поэтому на документ с десятками изображений приходятся десятки лишних `stat` и открытий корня. **Исправление:** разрешать корень один раз и переиспользовать `*os.Root` в пределах конвертации (ключ — путь корня).
  **Готово, когда:** для N активов выполняется одно разрешение корня; есть замер до/после.

- [x] **CLEAN-02 · P3: вынести `ReadWithin` из пакета `profile`.**

  **Код:** [internal/profile/styles.go](/Users/vyachin/Projects/go-pdf/internal/profile/styles.go); потребители — [internal/engine/images.go](/Users/vyachin/Projects/go-pdf/internal/engine/images.go), [internal/engine/fonts.go](/Users/vyachin/Projects/go-pdf/internal/engine/fonts.go).
  Пакет `profile` отвечает за правила документа, а `ReadWithin` — файловая утилита; её используют загрузчики изображений и шрифтов. **Исправление:** перенести функцию и `ErrOutsideRoot` в небольшой пакет (`internal/fsx` или `internal/assetfs`), оставив в `profile` только преобразование ошибок в диагностики `css/link-*`.
  **Готово, когда:** `profile` не содержит файловых примитивов, тесты границ остаются зелёными.

## Замечания, уже устранённые параллельными изменениями

- [x] **RESOLVED-01: фон блока перекрывал текст.** В исходном состоянии `<div style="background-color:#fff">VISIBLE</div>` давал 0 тёмных пикселей вместо 213 без фона. После изменений `finishFragment` повторная проверка дала 213 в обоих вариантах. Добавить или сохранить постоянный тест порядка отрисовки; исправление не сделано этим ревью.
- [x] **RESOLVED-02: компоненты RGB записывались в PDF как 0…255 вместо 0…1.** В актуальном `f3b` присутствует деление на 255. Повторная проверка диапазона PDF-цветов проходит; исправление не сделано этим ревью.

## Результаты проверки

Штатные тесты исходного состояния `go test ./...` прошли. На обновлённом отдельном снимке штатные тесты и `go vet ./...` также прошли. Дополнительные проверки ревью размещены вне рабочего дерева: 20 проверок, из них 16 выявили ошибки; некоторые проверки относятся к одной задаче выше. Проверки не добавлялись в исходники и не изменяют корпус.

При реализации переносить каждый подтверждённый сценарий в постоянный regression-тест. Успешный прогон нынешних тестов не закрывает перечисленные дефекты: большинство из них текущими проверками не обнаруживается.


## Review addendum — 2026-11-24

This addendum records **23 newly confirmed defects (one P1 and 22 P2)** and two additional maintenance improvements. The earlier checklist and its completion state are preserved. Source code was not changed by this review.

The first review snapshot exactly matches commit [02dffa5](https://github.com/vyachin/go-pdf/commit/02dffa5cb7bfa4aa2c2b113ff25fbb30bad3e10e). All findings below were reproduced again against a separate snapshot of `705497a`, after the concurrent CSS and flow fixes. File links below refer to that later state. The six earlier tasks CSS-01, CSS-03, CSS-04, CSS-05, LAY-03 and LAY-04 now pass their targeted checks and are not duplicated here.

Address **SEC-05 first**, then silent content loss (LAY-11, LAY-13), document structure (HTML-01), and PDF/font correctness. A successful existing test suite does not close these tasks: each needs a regression test for the specific reproduction.

### Process safety

- [x] **SEC-05 · P1: reject or bound table spans before integer conversion and layout.**

  **Source:** [internal/layout/table.go:132](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:132), `rowHeight`; [internal/layout/box.go:466](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:466), `attrInt`.
  **Reproduction:** `<table><tr><td rowspan="4294967296">A</td></tr></table>` on a 64-bit platform. Validation accepts it; layout panics with `runtime error: integer divide by zero` because converting the span to `int32` produces zero. This can terminate a service processing supplied HTML.
  **Fix / acceptance:** bound and validate `rowspan` and `colspan` before constructing the grid; use checked arithmetic for dimensions and allocations. This input must produce a bounded result or a diagnostic, without panic. Cover overflow values and very large column spans as well as ordinary spans.

### PDF and embedded fonts

- [x] **PDF-04 · P2: use identical glyph width rounding in `/W` and `TJ`.**

  **Source:** [internal/pdf/writer.go:334](/Users/vyachin/Projects/go-pdf/internal/pdf/writer.go:334), `widthArray`; [internal/pdf/writer.go:446](/Users/vyachin/Projects/go-pdf/internal/pdf/writer.go:446), `advanceAdjustment`.
  **Reproduction:** with Arial at 100 pt, glyph `W` has layout advance 94385 mp, `/W` contains 943, and the writer emits a zero adjustment. The viewer advances 94300 mp instead; the error accumulates within a word. `/W` truncates, while adjustment calculation rounds its supposed nominal width.
  **Fix / acceptance:** share one nominal-width calculation; verify effective PDF advances using the actual emitted `/W` values. Test a long repeated-glyph run at several sizes, including glyph widths with a fractional part above 0.5. Current kerning tests derive their expected nominal width with the same rounding assumption as the defective code.

- [x] **PDF-05 · P2: avoid multiplying PNG alpha twice when flattening.**

  **Source:** [internal/pdf/writer.go:501](/Users/vyachin/Projects/go-pdf/internal/pdf/writer.go:501), `imageObject`.
  **Reproduction:** a one-pixel NRGBA PNG with `(R,G,B,A)=(255,0,0,128)` is flattened to RGB `[191,127,127]`; compositing against white should produce `[255,127,127]`. Conversion to `image.RGBA` already premultiplies the channels, then the flattening formula multiplies them by alpha again. Translucent logos and antialiased edges become darker.
  **Fix / acceptance:** composite the decoded image onto an opaque white destination with `draw.Over`, or calculate from straight channels. Verify opaque, transparent and half-transparent pixels, plus edge pixels of a logo. This task concerns the documented white flattening policy; it does not require adding PDF transparency.

- [x] **PDF-06 · P2: preserve the color space of CMYK JPEG images.**

  **Source:** [internal/pdf/writer.go:480](/Users/vyachin/Projects/go-pdf/internal/pdf/writer.go:480), `imageObject`.
  **Reproduction:** Go's valid `src/image/testdata/video-001.cmyk.jpeg` has four components but is embedded unchanged with `/ColorSpace /DeviceRGB`. The descriptor disagrees with the JPEG data, causing incorrect rendering. The same branch handles every component count other than one as RGB.
  **Fix / acceptance:** support four-component JPEGs with the appropriate color space and Adobe inversion handling, or reject them explicitly before producing a PDF. Keep RGB/gray passthrough. Verify with a real CMYK fixture and an independent renderer, not only a synthetic JPEG header.

- [x] **FONT-01 · P2: leave the `head` directory checksum unchanged after setting `checkSumAdjustment`.**

  **Source:** [internal/font/subset.go:77](/Users/vyachin/Projects/go-pdf/internal/font/subset.go:77), `Subset`.
  **Reproduction:** subsetting Arial to `A` yields a whole-font checksum of `0xce3fefd2`, rather than `0xb1b0afba`. Updating the directory entry after computing the font adjustment changes the sum again, producing a structurally invalid embedded TrueType font even if a permissive viewer accepts it.
  **Fix / acceptance:** calculate the `head` table checksum with the adjustment field zero and retain that directory value after writing the adjustment, as specified in the [OpenType checksum rules](https://learn.microsoft.com/en-us/typography/opentype/spec/otff#calculating-checksums). Assert both table-directory checksums and the whole-font checksum on generated subsets.

- [x] **FONT-02 · P2: honor Coverage when applying class-based GPOS kerning.**

  **Source:** [internal/font/gpos.go:124](/Users/vyachin/Projects/go-pdf/internal/font/gpos.go:124), `parsePairPos`.
  **Reproduction:** a 48-byte PairPos format 2 subtable with `Coverage=[65]`, first-glyph classes `{65:1,66:1}`, second-glyph class `{86:1}`, and adjustment `(1,1)=-100` makes `Kerning(66,86)` return `-100`, although glyph 66 is outside Coverage. The parser reads Coverage but populates the class map from the entire ClassDef.
  **Fix / acceptance:** retain and enforce the covered first-glyph set for each class positioning subtable. Test covered and uncovered pairs sharing a class. Preserve subtable boundaries when representing these data; see the [PairPos format 2 definition](https://learn.microsoft.com/en-us/typography/opentype/spec/gpos#pair-adjustment-positioning-format-2-class-pair-adjustment).

- [x] **FONT-03 · P2: retain family aliases when deduplicating font files.**

  **Source:** [internal/engine/fonts.go:70](/Users/vyachin/Projects/go-pdf/internal/engine/fonts.go:70), `newFontRegistry`.
  **Reproduction:** declare `@font-face{font-family:One;src:url(f.ttf)}` and `@font-face{font-family:Two;src:url(f.ttf)}`. Only one face is registered because the deduplication key contains path, weight and italic state, but omits family. With a different configured fallback, text requesting `Two` selects the fallback instead of the declared font.
  **Fix / acceptance:** cache parsed font data by file identity while retaining each family/style registration. Both aliases must resolve to the declared file, and the file should still be read and parsed once.

### HTML structure

- [x] **HTML-01 · P2: make implicit end tags respect table and list scope.**

  **Source:** [internal/html/parse.go:238](/Users/vyachin/Projects/go-pdf/internal/html/parse.go:238), `implicitClose`; [internal/html/parse.go:300](/Users/vyachin/Projects/go-pdf/internal/html/parse.go:300), `closeOpen`.
  **Reproduction:** `<table id="outer"><tr><td><table id="inner"><tr><td>INNER</td></tr></table></td></tr></table>` leaves the inner table empty and attaches its row to the outer table. Similarly, `<ol><li id="outer">A<ol><li id="inner">B</li></ol>C</li></ol>` places the inner item outside the outer item. Unrestricted stack search closes an ancestor across the newly opened table/list boundary.
  **Fix / acceptance:** implement scoped closing for cells, rows, row groups and list items. Assert DOM parent relationships and resulting geometry for valid nested tables/lists, including trailing text. A text-only assertion is insufficient: `INNER` still appears in this defective output, but in the wrong table.

- [x] **HTML-02 · P2: keep a body-level stylesheet from becoming visible document text.**

  **Source:** [internal/css/ua.go:9](/Users/vyachin/Projects/go-pdf/internal/css/ua.go:9), `uaCSS`.
  **Reproduction:** `<p>A</p><style>p{color:red}</style><p>B</p>` passes the profile and renders `Ap{color:red}B`. Stylesheet collection uses the CSS, but the default hidden-elements rule omits `style`, so its source also participates in layout when the parser places it in the body.
  **Fix / acceptance:** exclude stylesheet nodes from rendering. Verify styles in the head and body still apply and neither is emitted as text or consumes document space.

### Layout and pagination

- [ ] **LAY-05 · P2: apply `display:none` to table groups, rows and cells.**

  **Source:** [internal/layout/box.go:392](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:392), `buildTable`.
  **Reproduction:** `<table><tr style="display:none"><td>HIDDEN</td></tr><tr><td>VISIBLE</td></tr></table>` renders `HIDDENVISIBLE`. The specialized table builder computes styles but never checks their display value.
  **Fix / acceptance:** skip hidden groups/rows/cells before building occupancy and widths. Verify hidden text is absent, consumes no space, and does not change visible spans or repeated headers.

- [ ] **LAY-06 · P2: do not override author styles on table headers after the cascade.**

  **Source:** [internal/layout/box.go:413](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:413), `buildTable`.
  **Reproduction:** `<style>th{font-weight:400;text-align:left}</style><table><tr><th>H</th></tr></table>` produces a cell with weight 700 and center alignment. Unless an inline style attribute exists, the builder overwrites the computed author values. The UA stylesheet already provides header defaults.
  **Fix / acceptance:** retain the computed style and keep defaults in the UA sheet. Test stylesheet overrides, class/id selectors and `!important`, as well as unstyled headers.

- [ ] **LAY-07 · P2: account for block margins when painting table-cell content.**

  **Source:** [internal/layout/table.go:281](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:281), `paintStaticBlocks` and `paintStaticBlock`.
  **Reproduction:** `<table><tr><td><p style="margin:0 0 20pt">A</p><p style="margin:0">B</p></td></tr></table>` places the baselines 14400 mp apart instead of at least 34400 mp. Measurement includes margins, but static painting advances only by child content height, so measured and drawn cell geometry disagree.
  **Fix / acceptance:** make cell measurement and painting share the same block-flow rules. Compare coordinates for multiple paragraphs with top/bottom margins and verify the row height encloses the painted content.

- [ ] **LAY-08 · P2: avoid a leading empty page for a break before the first content.**

  **Source:** [internal/layout/paginate.go:207](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:207), `placeBlock`; [internal/layout/paginate.go:242](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:242), `placeChild`.
  **Reproduction:** `<p style="break-before:page">FIRST</p>` creates two pages, with zero operations on the first. Entering the root sets `flow.empty=false` before any visible content, so the child's forced break generates an unnecessary page.
  **Fix / acceptance:** base emptiness on actual placed content and handle breaks in one consistent location. This input must have one page; a break after actual preceding content must still create the required next page. Include empty wrappers in the regression tests.

- [ ] **LAY-09 · P2: honor `widows` when the fitted count equals the requested minimum.**

  **Source:** [internal/layout/paginate.go:289](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:289), `placeText`.
  **Reproduction:** `@page{size:100pt 60pt;margin:10pt} p{margin:0;font-size:10pt;line-height:12pt;orphans:2;widows:3}` with `<p>A<br>B<br>C<br>D<br>E</p>` splits pages as `ABC / DE`. A valid `AB / CDE` split satisfies both constraints. The `fits > st.Widows` guard prevents this correction when `fits == 3`.
  **Fix / acceptance:** choose a split satisfying both orphan and widow constraints whenever possible. Test equality boundaries, first and continuation fragments, and genuinely impossible constraints separately.

- [ ] **LAY-10 · P2: register inline anchors for `target-counter`.**

  **Source:** [internal/layout/paginate.go:178](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:178), anchor registration; [internal/layout/lines.go:90](/Users/vyachin/Projects/go-pdf/internal/layout/lines.go:90), `collectPieces`.
  **Reproduction:** `<p><span id="target">TARGET</span></p>` draws the text but leaves `Document.Anchors` empty. An `a::after` rule referencing `#target` therefore displays 0 and reports an unknown target even though the element exists. Anchors are only recorded by paginated block placement.
  **Fix / acceptance:** carry inline element identity into positioned lines and register the page on which its content starts. Test inline targets before/after wrapping and across page breaks; include targets inside table cells.

- [x] **LAY-11 · P2: report missing glyphs in headers and footers instead of dropping their text.**

  **Source:** [internal/layout/margin.go:107](/Users/vyachin/Projects/go-pdf/internal/layout/margin.go:107), `paintMarginBoxes`.
  **Reproduction:** set a margin-box literal to a character absent from the selected font, for example U+10FFFF with Arial. `text.Shape` returns a missing-glyph error, but the code continues and conversion succeeds with no diagnostic and no header. Strict mode cannot detect an error that was discarded.
  **Fix / acceptance:** propagate a `text/glyph` diagnostic with the margin-box source context. Missing header/footer text must fail conversion consistently with body text; test normal and strict modes.

- [ ] **LAY-12 · P2: retain an image's aspect ratio when width is a percentage.**

  **Source:** [internal/layout/lines.go:279](/Users/vyachin/Projects/go-pdf/internal/layout/lines.go:279), `imageSize`.
  **Reproduction:** an intrinsically 80 pt by 40 pt image in a 200 pt container with `width:50%;height:auto` becomes 100 pt by 40 pt instead of 100 pt by 50 pt. The proportional-height branch only handles an absolute width; the percentage width was already resolved but is excluded by its original kind.
  **Fix / acceptance:** compute aspect ratio from the resolved specified dimension when the other dimension is auto. Test absolute and percentage widths, explicit width/height pairs, and percentage width with absolute height, which must not be overwritten.

- [x] **LAY-13 · P2: paint block images when mixed with block siblings.**

  **Source:** [internal/layout/paginate.go:116](/Users/vyachin/Projects/go-pdf/internal/layout/paginate.go:116), `placeBlock`; [internal/layout/box.go:338](/Users/vyachin/Projects/go-pdf/internal/layout/box.go:338), `buildElement`.
  **Reproduction:** `<p>BEFORE</p><img style="display:block" src="img.png"><p>AFTER</p>` with a valid PNG produces zero image operations and no error. The image becomes a standalone `Image` box, but block placement has no replaced-image branch and treats its empty children as an empty text block. An image-only document happens to pass through a different inline collection path.
  **Fix / acceptance:** place replaced block images with their resolved dimensions and correct flow advancement. Verify mixed siblings, an image-only document, and pagination near the bottom of a page; assert image operations, not just extracted text.

- [ ] **LAY-14 · P2: apply vertical alignment inside table cells.**

  **Source:** [internal/layout/table.go:232](/Users/vyachin/Projects/go-pdf/internal/layout/table.go:232), `paintCell`.
  **Reproduction:** `<table><tr><td style="vertical-align:bottom">BOTTOM</td><td>A<br>B<br>C</td></tr></table>` places `BOTTOM` on the first baseline, 28800 mp above `C`. `VerticalAlign` is parsed but the cell always paints at its top inset.
  **Fix / acceptance:** offset cell content using measured inner content height and available row height for top/middle/bottom alignment. Check unequal cell heights, padding and spans. If a value is intentionally unsupported, reject it instead of accepting it silently.

### CSS values and generated content

- [x] **CSS-06 · P2: convert percentage line height using the correct scale.**

  **Source:** [internal/css/props.go:188](/Users/vyachin/Projects/go-pdf/internal/css/props.go:188), the `line-height` parser.
  **Reproduction:** `<p style="line-height:100%;margin:0">A<br>B</p>` at the default 12 pt fails with `layout/no-progress`. `100%` is stored as 100000, divided by 1000 and converted to `100em`, yielding a 1200 pt line height instead of 12 pt. This remains broken after the unitless inheritance fix CSS-05.
  **Fix / acceptance:** divide by 100000 when converting this internal percentage representation into an em factor, or resolve percentage lengths directly. Assert used values for 100%, 120% and 150%, including inheritance at a different child font size.

- [ ] **CSS-07 · P2: apply specificity and importance to `a::after` content.**

  **Source:** [internal/css/cascade.go:60](/Users/vyachin/Projects/go-pdf/internal/css/cascade.go:60), `Compute`.
  **Reproduction:** `<style>a.hot::after{content:"CORRECT"}a::after{content:"WRONG"}</style><p><a class="hot">A</a></p>` renders `AWRONG`. Pseudo-element declarations are assigned during rule iteration before the normal cascade sorting, so later rules always win regardless of specificity or `!important`.
  **Fix / acceptance:** cascade generated content with the same importance, specificity and source-order rules used for other properties. Verify lower-specificity later rules, `!important`, and selector lists.

- [x] **CSS-08 · P2: resolve em-based border widths against the element font size.**

  **Source:** [internal/css/props.go:371](/Users/vyachin/Projects/go-pdf/internal/css/props.go:371), `parseBorderShorthand`; [internal/css/style.go:85](/Users/vyachin/Projects/go-pdf/internal/css/style.go:85), `resolveEm`.
  **Reproduction:** `<p style="border:1em solid black">A</p>` at 12 pt produces a border width of 100000 mp rather than 12000 mp. The border parser copies `Length.Value` into an absolute-width field and discards the unit; border widths never pass through font-relative resolution.
  **Fix / acceptance:** preserve the unit until computed-value resolution, or explicitly reject unsupported relative border widths. Check absolute and em widths at several font sizes; reject percentages/auto if not supported by the border grammar.

### Test infrastructure and maintenance

- [ ] **ENG-06 · P2: scope PDF text extraction maps to the active font.**

  **Source:** [internal/pdfdebug/extract.go:140](/Users/vyachin/Projects/go-pdf/internal/pdfdebug/extract.go:140), `buildToUni`, and `textFromStream`.
  **Reproduction:** write two text runs using Arial and Times New Roman with the same GID 30 but their respective characters U+037E and `;`. Extraction returns `;;` instead of `;;` (or duplicates the other character depending on map iteration). Every font's ToUnicode map is merged into one GID map, and `Tf` is ignored. Valid PDFs with multiple fonts therefore produce incorrect or unstable golden text.
  **Fix / acceptance:** resolve page font resources and switch the mapping on each `Tf`; preserve empty pages when exposing page-indexed text. Verify two real fonts with colliding GIDs and repeated extraction of the same bytes. Add independent text extraction in CI where practical.

- [ ] **ENG-07 · P3: separate elapsed-time performance budgets from ordinary and race tests.**

  **Source:** [internal/engine/bench_test.go:71](/Users/vyachin/Projects/go-pdf/internal/engine/bench_test.go:71), `TestPerformanceBudget`.
  The functional suite passes, but `go test -race ./...` failed this test because the 50-page conversion took about 1.07 seconds against an 800 ms wall-clock threshold. Race instrumentation and shared-machine load change these timings without proving a product regression.
  **Improvement / acceptance:** run fixed performance budgets in a dedicated, controlled job or opt-in benchmark mode. Keep page-count and content correctness assertions in the ordinary suite. The race job should exercise concurrency correctness without requiring uninstrumented speed; retain the separate performance gate.

- [ ] **CLI-01 · P3: pass the input filename to conversion diagnostics.**

  **Source:** [cmd/pdfgen/main.go:122](/Users/vyachin/Projects/go-pdf/cmd/pdfgen/main.go:122), `runConvert`.
  `runValidate` sets `Options.File`, but `runConvert` omits it, so inline HTML/CSS diagnostics identify `document.html` instead of the supplied input file. This makes CLI failures harder to locate and inconsistent with validation.
  **Improvement / acceptance:** set `File: input` and test a malformed template under a distinctive filename; its conversion diagnostic should identify that path and source coordinates.

**Additional evidence for existing ENG-02:** cancellation one millisecond after starting a 1500-paragraph conversion left `ctx.Err()==context.Canceled`, but conversion returned `err=nil` and a 38433-byte PDF. Thread cancellation through parsing, resource loading, layout and writing rather than checking only on entry. For resource limits, also cover span dimensions (SEC-05), expanded GPOS ranges and decoded image pixel counts before their expensive loops/allocations.

### Verification for this addendum

- The existing suite passed on the original checkout and on the later `705497a` snapshot. In snapshots containing the additional probes, `go test ./... -skip '^TestReview'` runs the original suite.
- `go vet ./...` passed on both review snapshots.
- The initial snapshot had **36 review checks: 32 failed, four passed**. After rechecking `705497a`, **26 failed and ten passed**; six previously reported CSS/flow cases were fixed concurrently. These are scenario counts, not distinct-defect counts: CMYK and nested structure have more than one check, and cancellation updates existing ENG-02. The 26 failures correspond to the 23 new defects above plus those overlaps and the existing cancellation issue.
- Both review snapshots passed `go test -race ./... -skip 'TestReview|TestPerformanceBudget'`; no race was reported. The unrestricted race run failed the elapsed-time budget described in ENG-07. An early run against actively edited working files hit an intermediate syntax error; it was not treated as a stable project defect.
- The real Arial font was available, and `/tmp/gobin/pdfcpu` existed, so the existing font-dependent tests and their independent PDF validation ran. Structural validation alone does not verify color reproduction, glyph positions or visual completeness.
- Temporary probes and full machine-readable results are outside the working tree: [initial results](/private/tmp/go-pdf-review-uv291h3c/review-results.jsonl), [latest results](/private/tmp/go-pdf-review-current-dacig99u/review-results.jsonl). The four `review_probe_test.go` files in each snapshot contain the executable reproductions. These temporary paths are local review evidence; the reproductions and acceptance criteria above are the durable handoff.
