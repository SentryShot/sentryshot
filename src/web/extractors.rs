use axum::extract::FromRequestParts;
use http::{header, request::Parts, StatusCode};

#[derive(Debug)]
pub enum Mp4StreamerRangeHeader {
    // bytes=S-E
    //StartEnd(u64, u64),

    // bytes=S-
    Start(u64),
    // bytes=-E
    //End(u64),
}

impl<S> FromRequestParts<S> for Mp4StreamerRangeHeader
where
    S: Send + Sync,
{
    type Rejection = (StatusCode, String);

    async fn from_request_parts(parts: &mut Parts, _: &S) -> Result<Self, Self::Rejection> {
        let Some(range) = parts.headers.get(header::RANGE) else {
            return Err((StatusCode::BAD_REQUEST, "missing range header".to_owned()));
        };
        let bad_req = |s: String| Err((StatusCode::BAD_REQUEST, s));
        let Ok(range) = range.to_str() else {
            return bad_req("range header is not a valid string".to_owned());
        };

        let mut chars = range.chars();
        if chars.by_ref().take(6).collect::<String>() != "bytes=" {
            return bad_req(format!("range header must start with 'bytes=': {range}"));
        }

        let start: String = chars.by_ref().take_while(char::is_ascii_digit).collect();
        if let Some(c) = chars.next() {
            if c != '-' {
                return bad_req(format!("unexpected char in range header: '{c}'"));
            }
        }

        let end: String = chars.by_ref().take_while(char::is_ascii_digit).collect();
        if let Some(c) = chars.next() {
            return bad_req(format!("unexpected char in range header: '{c}'"));
        }

        let parse_digits = |s: String| -> Result<u64, Self::Rejection> {
            s.parse().map_err(|_| {
                (
                    StatusCode::BAD_REQUEST,
                    format!("range header contains invalid number: {s}"),
                )
            })
        };

        match (!start.is_empty(), !end.is_empty()) {
            (true, true) => bad_req("range header start-end not implemented".to_owned()),
            (true, false) => Ok(Mp4StreamerRangeHeader::Start(parse_digits(start)?)),
            (false, true) => bad_req("range header -end not implemented".to_owned()),
            (false, false) => bad_req("range header bytes is empty".to_owned()),
        }
    }
}

#[cfg(test)]
#[allow(clippy::unwrap_used)]
mod tests {

    use super::*;
    use test_case::test_case;

    fn parts_with_range_header(range: &str) -> Parts {
        let mut builder = http::request::Builder::new();
        builder
            .headers_mut()
            .unwrap()
            .insert(header::RANGE, range.try_into().unwrap());
        builder.body(()).unwrap().into_parts().0
    }

    #[tokio::test]
    #[test_case("bytes=0-")]
    async fn test_parse_range(input: &str) {
        let mut parts = parts_with_range_header(input);
        Mp4StreamerRangeHeader::from_request_parts(&mut parts, &())
            .await
            .unwrap();
    }

    #[tokio::test]
    #[test_case("")]
    #[test_case("bytes=")]
    #[test_case("bytes=a")]
    #[test_case("bytes=0-a")]
    async fn test_parse_range_errors(input: &str) {
        let mut parts = parts_with_range_header(input);
        let (status, _) = Mp4StreamerRangeHeader::from_request_parts(&mut parts, &())
            .await
            .unwrap_err();
        assert_eq!(StatusCode::BAD_REQUEST, status);
    }
}
