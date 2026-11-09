//! A local stand-in for the API: records every request and answers each with what the test's handler says.
#![allow(dead_code)]

use std::io::Read;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use serde_json::{Map, Value};
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::net::TcpListener;

#[derive(Debug, Clone)]
pub struct Request {
    pub method: String,
    pub path: String,
    pub query: Option<String>,
    /// Names lowercased.
    pub headers: Vec<(String, String)>,
    /// As sent (gzipped when content-encoding says so).
    pub body: Vec<u8>,
}

impl Request {
    pub fn header(&self, name: &str) -> Option<&str> {
        self.headers
            .iter()
            .find(|(n, _)| n == name)
            .map(|(_, v)| v.as_str())
    }

    /// The body, gunzipped when content-encoding is gzip.
    pub fn decoded(&self) -> Vec<u8> {
        if self.header("content-encoding") != Some("gzip") {
            return self.body.clone();
        }
        let mut out = Vec::new();
        flate2::read::GzDecoder::new(&self.body[..])
            .read_to_end(&mut out)
            .expect("gzip body");
        out
    }

    pub fn json(&self) -> Map<String, Value> {
        serde_json::from_slice(&self.decoded()).expect("JSON object body")
    }

    /// The body's keys, sorted.
    pub fn keys(&self) -> Vec<String> {
        let mut keys: Vec<String> = self.json().keys().cloned().collect();
        keys.sort();
        keys
    }
}

pub struct Reply {
    pub status: u16,
    pub body: String,
    pub delay: Duration,
    /// Close the connection without answering.
    pub hang_up: bool,
}

pub fn reply(status: u16, body: &str) -> Reply {
    Reply {
        status,
        body: body.to_string(),
        delay: Duration::ZERO,
        hang_up: false,
    }
}

pub fn slow(delay: Duration) -> Reply {
    Reply {
        delay,
        ..reply(200, "{}")
    }
}

pub fn hang_up() -> Reply {
    Reply {
        hang_up: true,
        ..reply(200, "")
    }
}

pub struct Mock {
    pub url: String,
    requests: Arc<Mutex<Vec<Request>>>,
}

impl Mock {
    pub fn requests(&self) -> Vec<Request> {
        self.requests.lock().unwrap().clone()
    }

    pub fn count(&self) -> usize {
        self.requests.lock().unwrap().len()
    }

    pub fn last(&self) -> Request {
        self.requests().pop().expect("a request")
    }
}

/// Serves `handler(request, n)` (n: the requests before this one) on a local port until the test ends.
pub async fn serve<F>(handler: F) -> Mock
where
    F: Fn(&Request, usize) -> Reply + Send + Sync + 'static,
{
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let url = format!("http://{}", listener.local_addr().unwrap());
    let requests = Arc::new(Mutex::new(Vec::new()));
    let handler = Arc::new(handler);
    let seen = requests.clone();
    tokio::spawn(async move {
        loop {
            let Ok((stream, _)) = listener.accept().await else {
                return;
            };
            let (handler, seen) = (handler.clone(), seen.clone());
            tokio::spawn(async move {
                let mut stream = BufReader::new(stream);
                let Some(req) = read_request(&mut stream).await else {
                    return;
                };
                let n = {
                    let mut seen = seen.lock().unwrap();
                    seen.push(req.clone());
                    seen.len() - 1
                };
                let r = handler(&req, n);
                tokio::time::sleep(r.delay).await;
                if r.hang_up {
                    return;
                }
                let head = format!(
                    "HTTP/1.1 {} X\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n",
                    r.status,
                    r.body.len()
                );
                let _ = stream.write_all(head.as_bytes()).await;
                let _ = stream.write_all(r.body.as_bytes()).await;
                let _ = stream.shutdown().await;
            });
        }
    });
    Mock { url, requests }
}

async fn read_request(stream: &mut BufReader<tokio::net::TcpStream>) -> Option<Request> {
    let mut line = String::new();
    stream.read_line(&mut line).await.ok()?;
    let mut parts = line.split_whitespace();
    let method = parts.next()?.to_string();
    let target = parts.next()?;
    let (path, query) = match target.split_once('?') {
        Some((p, q)) => (p.to_string(), Some(q.to_string())),
        None => (target.to_string(), None),
    };
    let mut headers = Vec::new();
    loop {
        line.clear();
        stream.read_line(&mut line).await.ok()?;
        let l = line.trim_end();
        if l.is_empty() {
            break;
        }
        let (n, v) = l.split_once(':')?;
        headers.push((n.trim().to_lowercase(), v.trim().to_string()));
    }
    let len = headers
        .iter()
        .find(|(n, _)| n == "content-length")
        .and_then(|(_, v)| v.parse().ok())
        .unwrap_or(0);
    let mut body = vec![0; len];
    stream.read_exact(&mut body).await.ok()?;
    Some(Request {
        method,
        path,
        query,
        headers,
        body,
    })
}

/// The keys `want`, sorted, as owned strings.
pub fn keys(want: &[&str]) -> Vec<String> {
    let mut k: Vec<String> = want.iter().map(|s| s.to_string()).collect();
    k.sort();
    k
}
