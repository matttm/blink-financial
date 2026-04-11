use std::collections::VecDeque;
use std::env;
use std::time::{Duration, Instant};

use anyhow::Context;
use crossterm::event::{self, Event as CrosstermEvent, KeyCode, KeyEventKind};
use crossterm::execute;
use crossterm::terminal::{
    disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen,
};
use prost_types::Timestamp;
use ratatui::backend::CrosstermBackend;
use ratatui::layout::{Constraint, Direction, Layout, Margin, Rect};
use ratatui::style::{Color, Modifier, Style};
use ratatui::text::{Line, Span, Text};
use ratatui::widgets::{
    Block, Borders, Cell, Clear, List, ListItem, ListState, Paragraph, Row, Table, Wrap,
};
use ratatui::{Frame, Terminal};
use tokio::sync::mpsc;
use tokio::time::sleep;
use tonic::Request;

pub mod proto {
    tonic::include_proto!("blink.transactions.v1");
}

use proto::transaction_events_service_client::TransactionEventsServiceClient;
use proto::{StreamTransactionsRequest, Transaction, TransactionEvent};

const MAX_EVENTS: usize = 100;
const RECONNECT_DELAY: Duration = Duration::from_secs(30);

#[derive(Debug)]
enum UiEvent {
    StreamConnecting,
    StreamConnected,
    StreamDisconnected(String),
    Transaction(TransactionEvent),
}

enum ConnectionState {
    Connecting,
    Streaming,
    Retrying { next_retry_at: Instant },
}

impl Default for ConnectionState {
    fn default() -> Self {
        Self::Connecting
    }
}

#[derive(Default)]
struct App {
    endpoint: String,
    filters_summary: String,
    connection_state: ConnectionState,
    last_error: Option<String>,
    total_events: usize,
    events: VecDeque<TransactionEvent>,
    selected: usize,
}

impl App {
    fn new(endpoint: String, filters_summary: String) -> Self {
        Self {
            endpoint,
            filters_summary,
            connection_state: ConnectionState::Connecting,
            last_error: None,
            total_events: 0,
            events: VecDeque::with_capacity(MAX_EVENTS),
            selected: 0,
        }
    }

    fn apply_event(&mut self, event: UiEvent) {
        match event {
            UiEvent::StreamConnecting => {
                self.connection_state = ConnectionState::Connecting;
                self.last_error = None;
            }
            UiEvent::StreamConnected => {
                self.connection_state = ConnectionState::Streaming;
                self.last_error = None;
            }
            UiEvent::StreamDisconnected(error) => {
                self.connection_state = ConnectionState::Retrying {
                    next_retry_at: Instant::now() + RECONNECT_DELAY,
                };
                self.last_error = Some(error);
            }
            UiEvent::Transaction(event) => {
                self.connection_state = ConnectionState::Streaming;
                self.total_events += 1;
                self.events.push_front(event);
                if self.events.len() > MAX_EVENTS {
                    self.events.pop_back();
                }
                if self.selected >= self.events.len() {
                    self.selected = self.events.len().saturating_sub(1);
                }
            }
        }
    }

    fn next(&mut self) {
        if !self.events.is_empty() {
            self.selected = (self.selected + 1).min(self.events.len() - 1);
        }
    }

    fn previous(&mut self) {
        if !self.events.is_empty() {
            self.selected = self.selected.saturating_sub(1);
        }
    }

    fn selected_event(&self) -> Option<&TransactionEvent> {
        self.events.get(self.selected)
    }

    fn connection_label(&self) -> String {
        match self.connection_state {
            ConnectionState::Connecting => "connecting".to_string(),
            ConnectionState::Streaming => "connected".to_string(),
            ConnectionState::Retrying { next_retry_at } => {
                let remaining = next_retry_at
                    .saturating_duration_since(Instant::now())
                    .as_secs();
                format!("retrying in {}s", remaining)
            }
        }
    }

    fn connection_style(&self) -> Style {
        match self.connection_state {
            ConnectionState::Streaming => Style::default()
                .fg(Color::Green)
                .add_modifier(Modifier::BOLD),
            ConnectionState::Connecting => Style::default()
                .fg(Color::Yellow)
                .add_modifier(Modifier::BOLD),
            ConnectionState::Retrying { .. } => {
                Style::default().fg(Color::Red).add_modifier(Modifier::BOLD)
            }
        }
    }
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let config = TuiConfig::from_env();
    let (event_tx, mut event_rx) = mpsc::unbounded_channel();

    tokio::spawn(stream_events(config.clone(), event_tx));

    let mut terminal = setup_terminal()?;
    let app_result = run_app(
        &mut terminal,
        App::new(config.endpoint.clone(), config.filters.summary()),
        &mut event_rx,
    )
    .await;
    restore_terminal()?;
    app_result
}

async fn run_app(
    terminal: &mut Terminal<CrosstermBackend<std::io::Stdout>>,
    mut app: App,
    event_rx: &mut mpsc::UnboundedReceiver<UiEvent>,
) -> anyhow::Result<()> {
    let tick_rate = Duration::from_millis(150);
    let mut last_tick = Instant::now();

    loop {
        while let Ok(ui_event) = event_rx.try_recv() {
            app.apply_event(ui_event);
        }

        terminal.draw(|frame| draw(frame, &app))?;

        let timeout = tick_rate.saturating_sub(last_tick.elapsed());
        if event::poll(timeout)? {
            if let CrosstermEvent::Key(key) = event::read()? {
                if key.kind == KeyEventKind::Press {
                    match key.code {
                        KeyCode::Char('q') => return Ok(()),
                        KeyCode::Down | KeyCode::Char('j') => app.next(),
                        KeyCode::Up | KeyCode::Char('k') => app.previous(),
                        _ => {}
                    }
                }
            }
        }

        if last_tick.elapsed() >= tick_rate {
            last_tick = Instant::now();
        }
    }
}

fn draw(frame: &mut Frame<'_>, app: &App) {
    let chunks = Layout::default()
        .direction(Direction::Vertical)
        .constraints([
            Constraint::Length(4),
            Constraint::Min(10),
            Constraint::Length(2),
        ])
        .split(frame.area());

    draw_header(frame, chunks[0], app);

    let content = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([Constraint::Percentage(42), Constraint::Percentage(58)])
        .split(chunks[1]);

    draw_event_list(frame, content[0], app);
    draw_event_details(frame, content[1], app);

    let footer = Paragraph::new("q quit | j/k or arrows move selection")
        .block(Block::default().borders(Borders::TOP));
    frame.render_widget(footer, chunks[2]);
}

fn draw_header(frame: &mut Frame<'_>, area: Rect, app: &App) {
    let chunks = Layout::default()
        .direction(Direction::Horizontal)
        .constraints([Constraint::Percentage(70), Constraint::Percentage(30)])
        .split(area);

    let info_lines = vec![
        Line::from(Span::styled(
            "Blink Transaction Monitor",
            Style::default().add_modifier(Modifier::BOLD),
        )),
        Line::from(format!(
            "endpoint={} | filters={}",
            app.endpoint, app.filters_summary
        )),
        Line::from(format!(
            "total events={} | buffered={}",
            app.total_events,
            app.events.len()
        )),
    ];

    let info = Paragraph::new(Text::from(info_lines))
        .block(Block::default().borders(Borders::ALL).title("Stream"));
    frame.render_widget(info, chunks[0]);

    let mut status_lines = vec![Line::from(Span::styled(
        app.connection_label(),
        app.connection_style(),
    ))];

    status_lines.push(Line::from(
        app.last_error
            .as_ref()
            .map(|error| truncate(error, 48))
            .unwrap_or_else(|| "waiting for events".to_string()),
    ));

    let status = Paragraph::new(Text::from(status_lines))
        .block(Block::default().borders(Borders::ALL).title("Connection"))
        .wrap(Wrap { trim: true });
    frame.render_widget(status, chunks[1]);
}

fn draw_event_list(frame: &mut Frame<'_>, area: Rect, app: &App) {
    let items: Vec<ListItem<'_>> = app
        .events
        .iter()
        .map(|event| {
            let time = format_timestamp(event.received_at.as_ref());
            let preview = format!(
                "{} | {} | tx={} | {}",
                event.batch_id, event.source, event.transaction_count, time
            );
            ListItem::new(preview)
        })
        .collect();

    let list = List::new(items)
        .block(
            Block::default()
                .borders(Borders::ALL)
                .title("Recent Batches"),
        )
        .highlight_style(
            Style::default()
                .fg(Color::Black)
                .bg(Color::Cyan)
                .add_modifier(Modifier::BOLD),
        )
        .highlight_symbol("> ");

    let mut state = ListState::default();
    state.select(if app.events.is_empty() {
        None
    } else {
        Some(app.selected)
    });

    frame.render_stateful_widget(list, area, &mut state);
}

fn draw_event_details(frame: &mut Frame<'_>, area: Rect, app: &App) {
    let block = Block::default()
        .borders(Borders::ALL)
        .title("Selected Batch");
    frame.render_widget(block, area);

    let inner = area.inner(Margin {
        vertical: 1,
        horizontal: 1,
    });

    let Some(event) = app.selected_event() else {
        let empty = Paragraph::new("No streamed transactions yet.")
            .style(Style::default().fg(Color::DarkGray))
            .wrap(Wrap { trim: true });
        frame.render_widget(empty, inner);
        return;
    };

    let sections = Layout::default()
        .direction(Direction::Vertical)
        .constraints([Constraint::Length(7), Constraint::Min(10)])
        .split(inner);

    let summary = Paragraph::new(Text::from(vec![
        Line::from(format!("batch_id={}", event.batch_id)),
        Line::from(format!("source={}", event.source)),
        Line::from(format!("instance={}", event.instance)),
        Line::from(format!("transactions={}", event.transaction_count)),
        Line::from(format!(
            "received_at={}",
            format_timestamp(event.received_at.as_ref())
        )),
    ]))
    .wrap(Wrap { trim: true });
    frame.render_widget(summary, sections[0]);

    draw_transactions_table(frame, sections[1], &event.transactions);
}

fn draw_transactions_table(frame: &mut Frame<'_>, area: Rect, transactions: &[Transaction]) {
    let header = Row::new([
        Cell::from("Idempotency"),
        Cell::from("Tenant"),
        Cell::from("Account"),
        Cell::from("Type"),
        Cell::from("Amount"),
    ])
    .style(
        Style::default()
            .fg(Color::Yellow)
            .add_modifier(Modifier::BOLD),
    );

    let rows = transactions.iter().map(|tx| {
        let amount = tx
            .amount
            .as_ref()
            .map(|amount| format!("{} {}", amount.currency, amount.value))
            .unwrap_or_else(|| "-".to_string());
        Row::new([
            Cell::from(tx.idempotency_key.clone()),
            Cell::from(tx.tenant_id.clone()),
            Cell::from(tx.account_id.clone()),
            Cell::from(tx.r#type.clone()),
            Cell::from(amount),
        ])
    });

    let table = Table::new(
        rows,
        [
            Constraint::Percentage(28),
            Constraint::Percentage(16),
            Constraint::Percentage(16),
            Constraint::Length(10),
            Constraint::Percentage(30),
        ],
    )
    .header(header)
    .column_spacing(1)
    .block(Block::default().borders(Borders::TOP).title("Transactions"));

    frame.render_widget(Clear, area);
    frame.render_widget(table, area);
}

async fn stream_events(config: TuiConfig, sender: mpsc::UnboundedSender<UiEvent>) {
    loop {
        let _ = sender.send(UiEvent::StreamConnecting);

        let stream_result = async {
            let mut client = TransactionEventsServiceClient::connect(config.endpoint.clone())
                .await
                .with_context(|| format!("connect {}", config.endpoint))?;

            let request = Request::new(StreamTransactionsRequest {
                source: config.filters.source.clone(),
                tenant_id: config.filters.tenant_id.clone(),
                account_id: config.filters.account_id.clone(),
                batch_id: config.filters.batch_id.clone(),
            });

            let mut stream = client
                .stream_transactions(request)
                .await
                .context("open transaction event stream")?
                .into_inner();

            let _ = sender.send(UiEvent::StreamConnected);

            while let Some(event) = stream.message().await.context("read stream message")? {
                let _ = sender.send(UiEvent::Transaction(event));
            }

            anyhow::Ok(())
        }
        .await;

        match stream_result {
            Ok(()) => {
                let _ = sender.send(UiEvent::StreamDisconnected(
                    "stream closed; retry scheduled".to_string(),
                ));
            }
            Err(error) => {
                let _ = sender.send(UiEvent::StreamDisconnected(error.to_string()));
            }
        }

        sleep(RECONNECT_DELAY).await;
    }
}

fn setup_terminal() -> anyhow::Result<Terminal<CrosstermBackend<std::io::Stdout>>> {
    enable_raw_mode()?;
    let mut stdout = std::io::stdout();
    execute!(stdout, EnterAlternateScreen)?;
    let backend = CrosstermBackend::new(stdout);
    let terminal = Terminal::new(backend)?;
    Ok(terminal)
}

fn restore_terminal() -> anyhow::Result<()> {
    disable_raw_mode()?;
    execute!(std::io::stdout(), LeaveAlternateScreen)?;
    Ok(())
}

#[derive(Clone, Default)]
struct StreamFilters {
    source: String,
    tenant_id: String,
    account_id: String,
    batch_id: String,
}

impl StreamFilters {
    fn summary(&self) -> String {
        let mut parts = Vec::new();
        if !self.source.is_empty() {
            parts.push(format!("source={}", self.source));
        }
        if !self.tenant_id.is_empty() {
            parts.push(format!("tenant={}", self.tenant_id));
        }
        if !self.account_id.is_empty() {
            parts.push(format!("account={}", self.account_id));
        }
        if !self.batch_id.is_empty() {
            parts.push(format!("batch={}", self.batch_id));
        }
        if parts.is_empty() {
            "none".to_string()
        } else {
            parts.join(", ")
        }
    }
}

#[derive(Clone)]
struct TuiConfig {
    endpoint: String,
    filters: StreamFilters,
}

impl TuiConfig {
    fn from_env() -> Self {
        Self {
            endpoint: env::var("BLINK_GRPC_ENDPOINT")
                .unwrap_or_else(|_| "http://127.0.0.1:9091".to_string()),
            filters: StreamFilters {
                source: env::var("BLINK_STREAM_SOURCE").unwrap_or_default(),
                tenant_id: env::var("BLINK_STREAM_TENANT_ID").unwrap_or_default(),
                account_id: env::var("BLINK_STREAM_ACCOUNT_ID").unwrap_or_default(),
                batch_id: env::var("BLINK_STREAM_BATCH_ID").unwrap_or_default(),
            },
        }
    }
}

fn format_timestamp(timestamp: Option<&Timestamp>) -> String {
    match timestamp {
        Some(ts) => format!("{}.{:09}Z", ts.seconds, ts.nanos),
        None => "-".to_string(),
    }
}

fn truncate(value: &str, max_len: usize) -> String {
    if value.chars().count() <= max_len {
        value.to_string()
    } else {
        let truncated: String = value.chars().take(max_len.saturating_sub(3)).collect();
        format!("{}...", truncated)
    }
}
