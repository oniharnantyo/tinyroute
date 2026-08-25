## MODIFIED Requirements

### Requirement: Views are composed from templui components
All dashboard views (`view_*.templ`) SHALL build their UI from templui component packages (button, badge, card, input, table, selectbox, radio, breadcrumb, alert, skeleton, tabs, separator, dropdown, dialog, sheet, chart) instead of hand-rolled markup with ad-hoc Tailwind class chains for those primitives.

#### Scenario: A view renders a data table
- **WHEN** a view displays tabular data (keys, routes, history, failures, models)
- **THEN** the table is composed from `table.Table/Header/Body/Row/Head/Cell` rather than a hand-rolled `<table>` with utility classes

#### Scenario: A view renders a button
- **WHEN** a view displays a primary, secondary, destructive, outline, or ghost action
- **THEN** it uses `button.Button` with the corresponding typed variant constant (`button.VariantDefault`, `VariantSecondary`, `VariantDestructive`, `VariantOutline`, `VariantGhost`), never raw variant strings

#### Scenario: Variant props are typed constants
- **WHEN** any templui component accepts a variant or size
- **THEN** the value passed is the component package's exported constant, not a string literal

#### Scenario: A view renders a chart
- **WHEN** a view displays a data series over time
- **THEN** it composes the templui `chart` component rather than hand-rolled SVG or a third-party chart script, and the series data reaches the chart through server-rendered `data-*` attributes read from the DOM — script content SHALL NOT interpolate data values directly
