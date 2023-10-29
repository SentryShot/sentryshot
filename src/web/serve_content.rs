// SPDX-License-Identifier: GPL-2.0-or-later

use axum::{
    body::{boxed, Bytes, Full},
    response::{IntoResponse, Response},
};
use futures::Stream;
use http::{header, HeaderMap, HeaderName, HeaderValue, Method, StatusCode};
use http_body::Body;
use httpdate::HttpDate;
use pin_project::pin_project;
use std::{
    io::{SeekFrom, Write},
    pin::Pin,
    task::{Context, Poll},
    time::SystemTime,
};
use thiserror::Error;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncSeek, AsyncSeekExt, Take};
use tokio_util::io::ReaderStream;

// Replies to the request using the content in the
// provided ReadSeeker. The main benefit of serve_mp4_content over io::copy
// is that it handles Range requests properly, sets the MIME type, and
// handles If-Match, If-Unmodified-Since, If-None-Match, If-Modified-Since,
// and If-Range requests.
//
// If modtime is not the zero time or Unix epoch, serve_mp4_content
// includes it in a Last-Modified header in the response. If the
// request includes an If-Modified-Since header, serve_mp4_content uses
// modtime to decide whether the content needs to be sent at all.
//
// If the caller has set w's ETag header formatted per RFC 7232, section 2.3,
// ServeMP4Content uses it to handle requests using If-Match, If-None-Match, or If-Range.
//
// Content must be seeked to the beginning of the file.
pub async fn serve_mp4_content<RS>(
    method: &Method,
    headers: &HeaderMap,
    last_modified: std::time::SystemTime,
    size: u64,
    mut content: RS,
) -> Response
where
    RS: AsyncRead + AsyncSeek + Unpin + Send + Sync + 'static,
{
    let last_modified = LastModified::from(last_modified);
    let mut response_headers = HeaderMap::new();

    set_last_modified(&mut response_headers, &last_modified);

    let range_req =
        match check_preconditions(method, headers, response_headers.to_owned(), last_modified) {
            PreconditionsResult::Done(response) => return response,
            PreconditionsResult::Range(v) => v,
        };

    response_headers.insert(header::CONTENT_TYPE, HeaderValue::from_static("video/mp4"));

    let mut response_code = StatusCode::OK;

    // handle Content-Range header.
    let mut send_size = size;
    let mut ranges = match parse_range(range_req, size) {
        Ok(v) => v,
        Err(e) => {
            if matches!(e, ParseRangeError::NoOverlap) {
                response_headers.insert(
                    header::CONTENT_RANGE,
                    HeaderValue::from_str(&format!("bytes */{}", size)).unwrap(),
                );
            }
            return (
                StatusCode::RANGE_NOT_SATISFIABLE,
                response_headers,
                boxed(Full::from(e.to_string())),
            )
                .into_response();
        }
    };

    if sum_ranges_size(&ranges) > size {
        // The total number of bytes in all the ranges
        // is larger than the size of the file by
        // itself, so this is probably an attack, or a
        // dumb client. Ignore the range request.
        ranges = Vec::new();
    }

    if ranges.len() > 1 {
        return (
            StatusCode::RANGE_NOT_SATISFIABLE,
            response_headers,
            boxed(Full::from("Cannot serve multipart range requests")),
        )
            .into_response();
    }

    if ranges.len() == 1 {
        // RFC 7233, Section 4.1:
        // "If a single part is being transferred, the server
        // generating the 206 response MUST generate a
        // Content-Range header field, describing what range
        // of the selected representation is enclosed, and a
        // payload consisting of the range.
        // ...
        // A server MUST NOT generate a multipart response to
        // a request for a single range, since a client that
        // does not request multiple parts might not support
        // multipart responses."
        let ra = &ranges[0];
        if let Err(e) = content.seek(SeekFrom::Start(ra.start)).await {
            return (
                StatusCode::RANGE_NOT_SATISFIABLE,
                response_headers,
                boxed(Full::from(e.to_string())),
            )
                .into_response();
        }

        send_size = ra.length;
        response_code = StatusCode::PARTIAL_CONTENT;
        response_headers.insert(
            header::CONTENT_RANGE,
            HeaderValue::from_str(&ra.content_range(size)).unwrap(),
        );
    }

    response_headers.insert(header::ACCEPT_RANGES, HeaderValue::from_static("bytes"));
    if get_header(headers, header::CONTENT_ENCODING).is_none() {
        response_headers.insert(
            header::CONTENT_LENGTH,
            HeaderValue::from_str(&send_size.to_string()).unwrap(),
        );
    }
    /*w.Header().Set("Accept-Ranges", "bytes")
    if w.Header().Get("Content-Encoding") == "" {
        w.Header().Set("Content-Length", strconv.FormatInt(sendSize, 10))
    }*/

    //

    /*if size >= 0 { //nolint:nestif
        ranges, err := parseRange(rangeReq, size)
        if err != nil {
            //if errors.Is(err, errNoOverlap) {
                // /*w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
            //}
            http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
            return
        }
        if sumRangesSize(ranges) > size {
            // The total number of bytes in all the ranges
            // is larger than the size of the file by
            // itself, so this is probably an attack, or a
            // dumb client. Ignore the range request.
            ranges = nil
        }
        switch {
        case len(ranges) == 1:
            // RFC 7233, Section 4.1:
            // "If a single part is being transferred, the server
            // generating the 206 response MUST generate a
            // Content-Range header field, describing what range
            // of the selected representation is enclosed, and a
            // payload consisting of the range.
            // ...
            // A server MUST NOT generate a multipart response to
            // a request for a single range, since a client that
            // does not request multiple parts might not support
            // multipart responses."
            ra := ranges[0]
            if _, err := content.Seek(ra.start, io.SeekStart); err != nil {
                http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
                return
            }
            sendSize = ra.length
            code = http.StatusPartialContent
            w.Header().Set("Content-Range", ra.contentRange(size))
        case len(ranges) > 1:
            sendSize = rangesMIMESize(ranges, size)
            code = http.StatusPartialContent

            pr, pw := io.Pipe()
            mw := multipart.NewWriter(pw)
            w.Header().Set("Content-Type", "multipart/byteranges; boundary="+mw.Boundary())
            sendContent = pr
            defer pr.Close() // cause writing goroutine to fail and exit if CopyN doesn't finish.
            go func() {
                for _, ra := range ranges {
                    part, err := mw.CreatePart(ra.mimeHeader(size))
                    if err != nil {
                        pw.CloseWithError(err)
                        return
                    }
                    if _, err := content.Seek(ra.start, io.SeekStart); err != nil {
                        pw.CloseWithError(err)
                        return
                    }
                    if _, err := io.CopyN(part, content, ra.length); err != nil {
                        pw.CloseWithError(err)
                        return
                    }
                }
                mw.Close()
                pw.Close()
            }()
        }

        w.Header().Set("Accept-Ranges", "bytes")
        if w.Header().Get("Content-Encoding") == "" {
            w.Header().Set("Content-Length", strconv.FormatInt(sendSize, 10))
        }
    }

    w.WriteHeader(code)

    if r.Method != http.MethodHead {
        io.CopyN(w, sendContent, sendSize) //nolint:errcheck
    }*/

    let body = AsyncReadBody::limited(content, send_size).boxed();

    (response_code, response_headers, body).into_response()
}

enum PreconditionsResult {
    Done(Response),
    Range(Option<String>),
}

// Evaluates request preconditions and reports whether a precondition
// resulted in sending StatusNotModified or StatusPreconditionFailed.
fn check_preconditions(
    method: &Method,
    headers: &HeaderMap,
    response_headers: HeaderMap,
    last_modified: LastModified,
) -> PreconditionsResult {
    // This function carefully follows RFC 7232 section 6.
    let mut ch = check_if_match(headers);
    if ch == CondResult::None {
        ch = check_if_unmodified_since(headers, Some(&last_modified))
    }
    if ch == CondResult::False {
        return PreconditionsResult::Done(
            (
                StatusCode::PRECONDITION_FAILED,
                response_headers,
                boxed(Full::from("1")),
            )
                .into_response(),
        );
    }

    match check_if_none_match(headers) {
        CondResult::None => {
            if check_if_modified_since(method, headers, Some(&last_modified)) == CondResult::False {
                return PreconditionsResult::Done(not_modified_response(response_headers));
            }
        }
        CondResult::True => {}
        CondResult::False => {
            if method == Method::GET || method == Method::HEAD {
                return PreconditionsResult::Done(not_modified_response(response_headers));
            }
            return PreconditionsResult::Done(
                (
                    StatusCode::PRECONDITION_FAILED,
                    response_headers,
                    boxed(Full::from("if none match")),
                )
                    .into_response(),
            );
        }
    }

    let mut range_header = get_header(headers, header::RANGE);
    if range_header.is_some()
        && check_if_range(method, headers, Some(&last_modified)) == CondResult::False
    {
        range_header = None;
    }

    PreconditionsResult::Range(range_header)
}

// Determines if a syntactically valid ETag is present at s. If so,
// the ETag and remaining text after consuming ETag is returned.
// Otherwise, it returns "", "".
fn scan_etag(s: String) -> Option<(String, String)> {
    let s = trim_text_proto_string(Some(s));
    let mut start = 0;
    if let Some(s) = &s {
        if s.starts_with("W/") {
            start = 2;
        }
    }

    let Some(s) = s else {
        return None;
    };
    if s.as_bytes()[start..].len() < 2 || s.as_bytes()[start] != b'"' {
        return None;
    }

    // ETag is either W/"text" or "text".
    // See RFC 7232 2.3.
    for i in start + 1..s.len() {
        let c = s.as_bytes()[i];
        if
        // Character values allowed in ETags.
        c == 0x21 || (0x23..0x7e).contains(&c) || c >= 0x80 {
        } else if c == b'"' {
            return Some((
                String::from_utf8(s.as_bytes()[..i + 1].to_owned()).unwrap(),
                String::from_utf8(s.as_bytes()[i + 1..].to_owned()).unwrap(),
            ));
        } else {
            return None;
        }
    }

    None
}

// Reports whether a and b match using strong ETag comparison.
// Assumes a and b are valid ETags.
fn etag_strong_match(a: String, b: String) -> bool {
    a == b && !a.is_empty() && a.as_bytes()[0] == b'"'
}

// Reports whether a and b match using weak ETag comparison.
// Assumes a and b are valid ETags.
fn etag_weak_match(a: String, b: String) -> bool {
    a.strip_prefix("W/").unwrap_or(&a) == b.strip_prefix("W/").unwrap_or(&b)
}

// condResult is the result of an HTTP request precondition check.
// See https://tools.ietf.org/html/rfc7232 section 3.
#[derive(Debug, PartialEq)]
enum CondResult {
    None,
    True,
    False,
}

fn get_header(headers: &HeaderMap, name: HeaderName) -> Option<String> {
    let Some(header) = headers.get(name) else {
        return None
    };
    let Ok(header) = header.to_str() else {
        return None;
    };
    Some(header.to_owned())
}

fn check_if_match(headers: &HeaderMap) -> CondResult {
    let mut im = get_header(headers, header::IF_MATCH);
    if im.is_none() {
        return CondResult::None;
    }

    loop {
        im = trim_text_proto_string(im);
        let Some(im2) = im else {
            break;
        };
        if im2.as_bytes()[0] == b',' {
            im = Some(im2[1..].to_owned());
            continue;
        }
        if im2.as_bytes()[0] == b'*' {
            return CondResult::True;
        }
        let Some((etag, remain)) = scan_etag(im2) else {
            break;
        };

        let etag2 = get_header(headers, header::ETAG);
        if let Some(etag2) = etag2 {
            if etag_strong_match(etag, etag2) {
                return CondResult::True;
            }
        }
        im = Some(remain)
    }

    CondResult::False
}

fn check_if_unmodified_since(headers: &HeaderMap, modified: Option<&LastModified>) -> CondResult {
    let if_unmodified_since = headers
        .get(header::IF_UNMODIFIED_SINCE)
        .and_then(IfUnmodifiedSince::from_header_value);

    if if_unmodified_since.is_none() {
        return CondResult::None;
    }

    if let Some(since) = if_unmodified_since {
        let precondition = modified
            .as_ref()
            .map(|time| since.precondition_passes(time))
            .unwrap_or(false);

        if !precondition {
            return CondResult::False;
        }
        return CondResult::True;
    }
    CondResult::None
}

fn check_if_none_match(headers: &HeaderMap) -> CondResult {
    let Some(inm) = get_header(headers, header::IF_NONE_MATCH) else {
        return CondResult::None;
    };

    let mut buf = Some(inm);
    loop {
        buf = trim_text_proto_string(buf);
        let Some(buf2) = buf else {
            break;
        };

        if buf2.as_bytes()[0] == b',' {
            buf = Some(buf2[1..].to_string());
            continue;
        }

        if buf2.as_bytes()[0] == b'*' {
            return CondResult::False;
        }

        let Some((etag, remain)) = scan_etag(buf2) else {
            break;
        };

        if let Some(etag2) = get_header(headers, header::ETAG) {
            if etag_weak_match(etag, etag2) {
                return CondResult::False;
            }
        }

        buf = Some(remain)
    }
    CondResult::True
}

fn check_if_modified_since(
    method: &Method,
    headers: &HeaderMap,
    modified: Option<&LastModified>,
) -> CondResult {
    if method != Method::GET && method != Method::HEAD {
        return CondResult::None;
    }

    let if_modified_since = headers
        .get(header::IF_MODIFIED_SINCE)
        .and_then(IfModifiedSince::from_header_value);

    if let Some(since) = if_modified_since {
        let unmodified = modified
            .as_ref()
            .map(|time| !since.is_modified(time))
            // no last_modified means its always modified
            .unwrap_or(false);
        if unmodified {
            return CondResult::False;
        }
    }

    CondResult::True
}

fn check_if_range(
    method: &Method,
    headers: &HeaderMap,
    _modified: Option<&LastModified>,
) -> CondResult {
    if method != Method::GET && method != Method::HEAD {
        return CondResult::None;
    }

    let Some(ir) = get_header(headers,header::IF_RANGE) else {
        return CondResult::None;
    };

    if let Some((etag, _)) = scan_etag(ir) {
        if let Some(etag2) = get_header(headers, header::ETAG) {
            if etag_strong_match(etag, etag2) {
                return CondResult::True;
            }
        }
        return CondResult::False;
    };

    // The If-Range value is typically the ETag value, but it may also be
    // the modtime date. See golang.org/issue/8367.
    /*if modtime.IsZero() {
        return condFalse
    }
    t, err := http.ParseTime(ir)
    if err != nil {
        return condFalse
    }
    if t.Unix() == modtime.Unix() {
        return condTrue
    }*/
    CondResult::False
}

struct LastModified(HttpDate);

impl From<SystemTime> for LastModified {
    fn from(time: SystemTime) -> Self {
        LastModified(time.into())
    }
}

struct IfModifiedSince(HttpDate);

impl IfModifiedSince {
    /// Check if the supplied time means the resource has been modified.
    fn is_modified(&self, last_modified: &LastModified) -> bool {
        self.0 < last_modified.0
    }

    /// convert a header value into a IfModifiedSince, invalid values are silentely ignored
    fn from_header_value(value: &HeaderValue) -> Option<IfModifiedSince> {
        std::str::from_utf8(value.as_bytes())
            .ok()
            .and_then(|value| httpdate::parse_http_date(value).ok())
            .map(|time| IfModifiedSince(time.into()))
    }
}

struct IfUnmodifiedSince(HttpDate);

impl IfUnmodifiedSince {
    /// Check if the supplied time passes the precondtion.
    fn precondition_passes(&self, last_modified: &LastModified) -> bool {
        self.0 >= last_modified.0
    }

    /// Convert a header value into a IfModifiedSince, invalid values are silentely ignored
    fn from_header_value(value: &HeaderValue) -> Option<IfUnmodifiedSince> {
        std::str::from_utf8(value.as_bytes())
            .ok()
            .and_then(|value| httpdate::parse_http_date(value).ok())
            .map(|time| IfUnmodifiedSince(time.into()))
    }
}

fn set_last_modified(headers: &mut HeaderMap, modtime: &LastModified) {
    headers.insert(
        header::LAST_MODIFIED,
        HeaderValue::from_str(&modtime.0.to_string()).unwrap(),
    );
}

fn not_modified_response(mut response_headers: HeaderMap) -> Response {
    // RFC 7232 section 4.1:
    // a sender SHOULD NOT generate representation metadata other than the
    // above listed fields unless said metadata exists for the purpose of
    // guiding cache updates (e.g., Last-Modified might be useful if the
    // response does not have an ETag field).
    response_headers.remove(header::CONTENT_TYPE);
    response_headers.remove(header::CONTENT_LENGTH);
    response_headers.remove(header::CONTENT_ENCODING);
    if response_headers.get(header::ETAG).is_some() {
        response_headers.remove(header::LAST_MODIFIED);
    }

    (StatusCode::NOT_MODIFIED, response_headers).into_response()
}

// Specifies the byte range to be sent to the client.
#[derive(Debug)]
struct HttpRange {
    start: u64,
    length: u64,
}

impl HttpRange {
    fn content_range(&self, size: u64) -> String {
        format!(
            "bytes {}-{}/{}",
            self.start,
            self.start + self.length - 1,
            size,
        )
    }

    /*fn mime_header(&self, size: u64) -> HashMap<String, Vec<String>> {
        HashMap::from([
            ("Content-Range".to_owned(), vec![self.content_range(size)]),
            ("Content-Type".to_owned(), vec!["video/mp4".to_owned()]),
        ])
        /*return textproto.MIMEHeader{
            "Content-Range": {r.contentRange(size)},
            "Content-Type":  {"video/mp4"},
        }*/
    }*/
}

#[derive(Debug, Error)]
enum ParseRangeError {
    #[error("invalid range")]
    InvalidRange,

    // If first-byte-pos of all of the byte-range-spec values is greater than the content size.
    #[error("invalid range: failed to overlap")]
    NoOverlap,
}

// parses a Range header string as per RFC 7233.
// errNoOverlap is returned if none of the ranges overlap.
fn parse_range(s: Option<String>, size: u64) -> Result<Vec<HttpRange>, ParseRangeError> {
    use ParseRangeError::*;
    let Some(s) = s else {
        return Ok(Vec::new());
    };

    const B: &str = "bytes=";

    if !s.starts_with(B) {
        return Err(InvalidRange);
    }

    let mut ranges = Vec::new();
    let mut no_overlap = false;

    for ra in s[B.len()..].split(',') {
        let ra = trim_text_proto_string(Some(ra.to_owned()));
        let Some(ra) = ra else {
            continue
        };

        let i = ra.chars().position(|c| c == '-').ok_or(InvalidRange)?;

        let start = trim_text_proto_string(Some(ra[..i].to_owned()));
        let end = trim_text_proto_string(Some(ra[i + 1..].to_owned()));

        let mut r = HttpRange {
            start: 0,
            length: 0,
        };

        if let Some(start) = start {
            let i: u64 = start.parse().map_err(|_| InvalidRange)?;
            if i >= size {
                // If the range begins after the size of the content,
                // then it does not overlap.
                no_overlap = true;
                continue;
            }
            r.start = i;
            if let Some(end) = end {
                let mut i: u64 = end.parse().map_err(|_| InvalidRange)?;
                if r.start > i {
                    return Err(InvalidRange);
                }
                if i >= size {
                    i = size - 1
                }
                r.length = i - r.start + 1
            } else {
                // If no end is specified, range extends to end of the file.
                r.length = size - r.start
            }
        } else {
            // If no start is specified, end specifies the
            // range start relative to the end of the file,
            // and we are dealing with <suffix-length>
            // which has to be a non-negative integer as per
            // RFC 7233 Section 2.1 "Byte-Ranges".
            let Some(end) = end else {
                return Err(InvalidRange);
            };
            if end.as_bytes()[0] == b'-' {
                return Err(InvalidRange);
            }
            let mut i: u64 = end.parse().map_err(|_| InvalidRange)?;
            if i > size {
                i = size
            }
            r.start = size - i;
            r.length = size - r.start;
            /*
            if end == "" || end[0] == '-' {
                return nil, errors.New("invalid range") //nolint:goerr113
            }
            i, err := strconv.ParseInt(end, 10, 64)
            if i < 0 || err != nil {
                return nil, errors.New("invalid range") //nolint:goerr113
            }
            if i > size {
                i = size
            }
            r.start = size - i
            r.length = size - r.start
            */
        }

        ranges.push(r);
    }

    if no_overlap && ranges.is_empty() {
        // The specified ranges did not overlap with the content.
        return Err(NoOverlap);
    }

    Ok(ranges)
}

fn trim_text_proto_string(s: Option<String>) -> Option<String> {
    let Some(mut s) = s else {
        return None
    };
    while !s.is_empty() && is_ascii_space(s.as_bytes()[0]) {
        s = s[1..].to_string();
    }
    while !s.is_empty() && is_ascii_space(*s.as_bytes().last().unwrap()) {
        s = s.as_bytes().last().unwrap().to_string()
    }
    if s.is_empty() {
        return None;
    }
    Some(s)
}

fn is_ascii_space(b: u8) -> bool {
    b == b' ' || b == b'\t' || b == b'\n' || b == b'\r'
}

// Counts how many bytes have been written to it.
struct CountingWriter(u64);

impl Write for CountingWriter {
    fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
        self.0 += u64::try_from(buf.len()).unwrap();
        Ok(buf.len())
    }

    fn flush(&mut self) -> std::io::Result<()> {
        Ok(())
    }
}

/*func (w *countingWriter) Write(p []byte) (int, error) {
    *w += countingWriter(len(p))
    return len(p), nil
}*/

// Returns the number of bytes it takes to encode
// the provided ranges as a multipart response.
/*fn ranges_mime_size(ranges: &Vec<HttpRange>, content_size: u64) -> u64 {
    let mut enc_size = 0;
    let mut w = CountingWriter(0);
    let mut mw = MultiPartWriter::new();
    //mw := multipart.NewWriter(&w)
    for ra in ranges {
        _ = mw.create_part(ra.mime_header(content_size), &mut w);
        enc_size += ra.length;
    }
    /*for _, ra := range ranges {
        mw.CreatePart(ra.mimeHeader(contentSize)) //nolint:errcheck
        encSize += ra.length
    }*/
    enc_size += w.0;
    //encSize += int64(w)
    //return encSize*//*
    enc_size
}*/

fn sum_ranges_size(ranges: &Vec<HttpRange>) -> u64 {
    let mut size = 0;
    for ra in ranges {
        size += ra.length;
    }
    size
}

/*
struct MultiPartWriter {
    boundary: String,
    first_part: bool,
}

impl MultiPartWriter {
    fn new() -> Self {
        Self {
            boundary: random_boundary(),
            first_part: true,
        }
    }
    // CreatePart creates a new multipart section with the provided
    // header. The body of the part should be written to the returned
    // Writer. After calling CreatePart, any previous part may no longer
    // be written to.
    fn create_part(
        &mut self,
        header: HashMap<String, Vec<String>>,
        w: &mut impl Write,
    ) -> Result<(), CreatePartError> {
        use CreatePartError::*;
        /*if w.lastpart != nil {
            if err := w.lastpart.close(); err != nil {
                return nil, err
            }
        }*/

        let mut b = Vec::new();
        //var b bytes.Buffer
        if !self.first_part {
            write!(b, "\r\n--{}\r\n", self.boundary)?;
        } else {
            write!(b, "--{}\r\n", self.boundary)?;
        }
        /*if w.lastpart != nil {
            fmt.Fprintf(&b, "\r\n--%s\r\n", w.boundary)
        } else {
            fmt.Fprintf(&b, "--%s\r\n", w.boundary)
        }*/

        let mut keys: Vec<String> = Vec::with_capacity(header.len());
        for k in header.keys() {
            keys.push(k.to_owned());
        }
        /*
        keys := make([]string, 0, len(header))
        for k := range header {
            keys = append(keys, k)
        }
        */

        keys.sort();
        //sort.Strings(keys)

        for k in keys {
            for v in header.get(&k).unwrap() {
                write!(b, "{}: {}\r\n", k, v)?;
            }
        }
        /*for _, k := range keys {
            for _, v := range header[k] {
                fmt.Fprintf(&b, "%s: %s\r\n", k, v)
            }
        }*/

        write!(b, "\r\n")?;
        //fmt.Fprintf(&b, "\r\n")
        std::io::copy(&mut Cursor::new(b), w).map_err(Copy)?;
        /*_, err := io.Copy(w.w, &b)
        if err != nil {
            return nil, err
        }
        */

        /*
        p := &part{
            mw: w,
        }
        */
        //w.lastpart = p
        //return p, nil
        Ok(())
    }
}

#[derive(Debug, Error)]
enum CreatePartError {
    #[error("write: {0}")]
    Write(#[from] std::io::Error),

    #[error("copy {0}")]
    Copy(std::io::Error),
}

fn random_boundary() -> String {
    let rng = rand::thread_rng();
    let s: String = rng
        .sample_iter(Alphanumeric)
        .take(30)
        .map(char::from)
        .collect();
    /*var buf [30]byte
    _, err := io.ReadFull(rand.Reader, buf[:])
    if err != nil {
        panic(err)
    }
    return fmt.Sprintf("%x", buf[:])*/
    s
}*/

/// Adapter that turns an [`impl AsyncRead`][tokio::io::AsyncRead] to an [`impl Body`][http_body::Body].
#[derive(Debug)]
#[pin_project]
pub struct AsyncReadBody<T> {
    #[pin]
    reader: ReaderStream<T>,
}

const CAPACITY: usize = 65536;

impl<T> AsyncReadBody<T>
where
    T: AsyncRead,
{
    fn limited(read: T, max_read_bytes: u64) -> AsyncReadBody<Take<T>> {
        AsyncReadBody {
            reader: ReaderStream::with_capacity(read.take(max_read_bytes), CAPACITY),
        }
    }
}

impl<T> Body for AsyncReadBody<T>
where
    T: AsyncRead,
{
    type Data = Bytes;
    type Error = std::io::Error;

    fn poll_data(
        self: Pin<&mut Self>,
        cx: &mut Context<'_>,
    ) -> Poll<Option<Result<Self::Data, Self::Error>>> {
        self.project().reader.poll_next(cx)
    }

    fn poll_trailers(
        self: Pin<&mut Self>,
        _cx: &mut Context<'_>,
    ) -> Poll<Result<Option<HeaderMap>, Self::Error>> {
        Poll::Ready(Ok(None))
    }
}
