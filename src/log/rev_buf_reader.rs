// SPDX-License-Identifier: GPL-2.0-or-later

use pin_project::pin_project;
use std::{
    io::SeekFrom,
    pin::Pin,
    task::{Context, Poll},
};
use tokio::io::{AsyncBufRead, AsyncRead, AsyncSeek, ReadBuf};

/// The `RevBufReader` struct adds reverse buffering to any readseeker.
///
/// It can be excessively inefficient to work directly with a [`AsyncRead`]
/// instance. A `RevBufReader` performs large, infrequent reads on the underlying
/// [`AsyncRead`] and maintains an in-memory buffer of the results.
///
/// `RevBufReader` can improve the speed of programs that make *small* and
/// *repeated* read calls to the same file. It does not help when reading
/// very  large amounts at once, or reading just one or a few times.
/// It also provides no advantage when reading from a source that is
/// already in memory, like a `Vec<u8>`.
///
/// `RevBufReader` will seek backwards before refilling the buffer if the
/// previous seek was backwards
/// Unlike `tokio::io::BufReader`, the internal buffer does not get cleared when seeking.
#[pin_project]
pub(crate) struct RevBufReader<R> {
    #[pin]
    inner: R,
    inner_pos: usize,
    buf: Box<[u8]>,
    buf_start: usize,
    buf_pos: usize,
    buf_end: usize,
    seek_pos: usize,
    prev_seek_pos: usize,
    read_state: ReadState,
    seek_state: SeekState,
}

impl<R: AsyncRead + AsyncSeek> RevBufReader<R> {
    /// Creates a new `RevBufReader` with a default buffer capacity. The default is currently 32KIB,
    /// but may change in the future.
    pub(crate) fn new(inner: R) -> Self {
        Self::with_capacity(32768, inner)
    }

    /// Creates a new `RevBufReader` with the specified buffer capacity.
    pub(crate) fn with_capacity(capacity: usize, inner: R) -> Self {
        Self {
            inner,
            inner_pos: 0,
            buf: vec![0; capacity].into_boxed_slice(),
            buf_start: 0,
            buf_pos: 0,
            buf_end: 0,
            seek_pos: 0,
            prev_seek_pos: 0,
            read_state: ReadState::Init,
            seek_state: SeekState::Init,
        }
    }

    /// Gets a reference to the underlying reader.
    ///
    /// It is inadvisable to directly read from the underlying reader.
    #[cfg(test)]
    fn get_ref(&self) -> &R {
        &self.inner
    }

    /// Returns a reference to the internally buffered data.
    ///
    /// Unlike `fill_buf`, this will not attempt to fill the buffer if it is empty.
    #[cfg(test)]
    fn buffer(&self) -> &[u8] {
        if self.buf_pos < self.buf_start || self.buf_end <= self.buf_pos {
            return &[];
        }
        &self.buf[(self.buf_pos - self.buf_start)..(self.buf_end - self.buf_start)]
    }
}

macro_rules! ready {
    ($e:expr $(,)?) => {
        match $e {
            std::task::Poll::Ready(t) => t,
            std::task::Poll::Pending => return std::task::Poll::Pending,
        }
    };
}

impl<R: AsyncRead + AsyncSeek> AsyncRead for RevBufReader<R> {
    fn poll_read(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut ReadBuf<'_>,
    ) -> Poll<std::io::Result<()>> {
        // If our position matches the inner buffer and we're doing a massive read
        // (larger than our internal buffer), bypass our internal buffer entirely.
        if self.buf_pos == self.inner_pos && buf.remaining() >= self.buf.len() {
            let res = ready!(self.as_mut().project().inner.poll_read(cx, buf));
            *self.as_mut().project().buf_pos += buf.filled().len();
            *self.as_mut().project().inner_pos += buf.filled().len();
            return Poll::Ready(res);
        }
        let rem = ready!(self.as_mut().poll_fill_buf(cx))?;
        let amt = std::cmp::min(rem.len(), buf.remaining());
        buf.put_slice(&rem[..amt]);
        self.consume(amt);
        Poll::Ready(Ok(()))
    }
}

enum ReadState {
    /// `poll_fill_buf` has not been called.
    Init,
    // start_seek has been called and poll_complete is pending.
    Seek(usize),
    // poll_read is pending.
    Read(usize),
}

impl<R: AsyncRead + AsyncSeek> AsyncBufRead for RevBufReader<R> {
    fn poll_fill_buf<'a>(
        mut self: Pin<&'a mut Self>,
        cx: &mut Context<'_>,
    ) -> Poll<std::io::Result<&'a [u8]>> {
        let read = |s: Pin<&'a mut Self>,
                    cx: &mut Context,
                    new_pos: usize|
         -> Poll<std::io::Result<&[u8]>> {
            let me = s.project();
            let mut buf = ReadBuf::new(me.buf);
            if me.inner.poll_read(cx, &mut buf)?.is_pending() {
                *me.read_state = ReadState::Read(new_pos);
                return Poll::Pending;
            }
            *me.buf_start = new_pos;
            *me.buf_end = *me.buf_start + buf.filled().len();
            *me.inner_pos = *me.buf_end;

            if *me.buf_end <= *me.buf_pos {
                // Cursor is outside the file.
                *me.read_state = ReadState::Init;
                return Poll::Ready(Ok(&[])); // EOF.
            }

            *me.read_state = ReadState::Init;
            Poll::Ready(Ok(
                &me.buf[(*me.buf_pos - *me.buf_start)..(*me.buf_end - *me.buf_start)]
            ))
        };
        let seek = |mut s: Pin<&'a mut Self>,
                    cx: &mut Context,
                    new_pos: usize|
         -> Poll<std::io::Result<&[u8]>> {
            match s.as_mut().project().inner.poll_complete(cx)? {
                Poll::Ready(_) => read(s, cx, new_pos),
                Poll::Pending => {
                    *s.as_mut().project().read_state = ReadState::Seek(new_pos);
                    Poll::Pending
                }
            }
        };

        match self.read_state {
            ReadState::Init => {
                // If we're outside the range our internal buffer then we need
                // to fetch some more data. We may first need seek to either move
                // back to the end of the prevous buffer or to buffer backwards.
                let me = self.as_mut().project();
                if *me.buf_pos < *me.buf_start || *me.buf_end <= *me.buf_pos {
                    let back_seek = me.seek_pos < me.prev_seek_pos;
                    let prev_pos_minus_cap = (*me.prev_seek_pos).saturating_sub(me.buf.len());
                    let new_pos = if back_seek && prev_pos_minus_cap <= *me.buf_pos {
                        prev_pos_minus_cap
                    } else {
                        *me.buf_pos
                    };
                    // Skip seeking if the position didn't change.
                    if new_pos == *me.inner_pos {
                        return read(self, cx, new_pos);
                    }

                    #[allow(clippy::as_conversions)]
                    me.inner.start_seek(SeekFrom::Start(new_pos as u64))?;
                    return seek(self, cx, new_pos);
                }

                let me = self.project();
                Poll::Ready(Ok(
                    &me.buf[(*me.buf_pos - *me.buf_start)..(*me.buf_end - *me.buf_start)]
                ))
            }
            ReadState::Seek(new_pos) => seek(self, cx, new_pos),
            ReadState::Read(new_pos) => read(self, cx, new_pos),
        }
    }

    fn consume(self: Pin<&mut Self>, amt: usize) {
        let me = self.project();
        *me.buf_pos = std::cmp::min(*me.buf_pos + amt, *me.buf_end);
    }
}

#[derive(Debug, Clone, Copy)]
enum SeekState {
    /// `start_seek` has not been called.
    Init,
    /// `start_seek` has been called, but `poll_complete` has not yet been called.
    Start(SeekFrom),
}

/// Seeks to an offset, in bytes, in the underlying reader.
///
/// See [`AsyncSeek`] for more details.
impl<R: AsyncRead + AsyncSeek> AsyncSeek for RevBufReader<R> {
    fn start_seek(self: Pin<&mut Self>, pos: SeekFrom) -> std::io::Result<()> {
        // We needs to call seek operation multiple times.
        // And we should always call both start_seek and poll_complete,
        // as start_seek alone cannot guarantee that the operation will be completed.
        // poll_complete receives a Context and returns a Poll, so it cannot be called
        // inside start_seek.
        *self.project().seek_state = SeekState::Start(pos);
        Ok(())
    }

    fn poll_complete(mut self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<std::io::Result<u64>> {
        match self.seek_state {
            SeekState::Init => {
                // 1.x AsyncSeek recommends calling poll_complete before start_seek.
                // We don't have to guarantee that the value returned by
                // poll_complete called without start_seek is correct,
                // so we'll return 0.
                Poll::Ready(Ok(0))
            }
            SeekState::Start(SeekFrom::Start(pos)) => {
                let me = self.as_mut().project();
                *me.buf_pos = usize::try_from(pos).expect("seek pos should fit usize");
                *me.prev_seek_pos = *me.seek_pos;
                *me.seek_pos = *me.buf_pos;
                *me.seek_state = SeekState::Init;
                Poll::Ready(Ok(pos))
            }
            SeekState::Start(_) => {
                panic!("can only seek from start")
            }
        }
    }
}

// https://github.com/tokio-rs/tokio/blob/master/tokio/tests/io_buf_reader.rs
#[allow(
    clippy::cast_sign_loss,
    clippy::cast_possible_truncation,
    clippy::as_conversions,
    clippy::unwrap_used
)]
#[cfg(test)]
mod tests {
    use super::*;
    use std::{
        cmp,
        io::Cursor,
        ptr::null,
        task::{Poll, RawWaker, RawWakerVTable, Waker},
    };
    use tokio::io::{AsyncBufRead, AsyncBufReadExt, AsyncReadExt, AsyncSeekExt};

    #[tokio::test]
    async fn test_seek_outside_file() {
        let mut buf = Cursor::new(vec![0, 1, 2, 3, 4, 5, 6, 7, 8, 9]);
        let mut reader = RevBufReader::new(&mut buf);

        reader.seek(SeekFrom::Start(14)).await.unwrap();
        reader.seek(SeekFrom::Start(11)).await.unwrap();
        reader.read_exact(&mut [0]).await.unwrap_err();

        // Seek back into the file.
        reader.seek(SeekFrom::Start(5)).await.unwrap();
        let mut tmp = vec![0, 0, 0, 0];
        reader.read_exact(&mut tmp).await.unwrap();
        assert_eq!(&[5, 6, 7, 8], tmp.as_slice());

        let mut tmp = vec![0];
        reader.read_exact(&mut tmp).await.unwrap();
        assert_eq!(&[9], tmp.as_slice());
    }

    unsafe fn noop_clone(_data: *const ()) -> RawWaker {
        noop_raw_waker()
    }

    unsafe fn noop(_data: *const ()) {}

    const NOOP_WAKER_VTABLE: RawWakerVTable = RawWakerVTable::new(noop_clone, noop, noop, noop);

    const fn noop_raw_waker() -> RawWaker {
        RawWaker::new(null(), &NOOP_WAKER_VTABLE)
    }

    #[inline]
    #[allow(clippy::ptr_as_ptr)]
    pub fn noop_waker_ref() -> &'static Waker {
        struct SyncRawWaker(RawWaker);
        unsafe impl Sync for SyncRawWaker {}

        static NOOP_WAKER_INSTANCE: SyncRawWaker = SyncRawWaker(noop_raw_waker());

        // SAFETY: `Waker` is #[repr(transparent)] over its `RawWaker`.
        unsafe { &*(std::ptr::addr_of!(NOOP_WAKER_INSTANCE.0) as *const Waker) }
    }

    macro_rules! run_fill_buf {
        ($reader:expr) => {{
            let mut cx = Context::from_waker(noop_waker_ref());
            loop {
                if let Poll::Ready(x) = Pin::new(&mut $reader).poll_fill_buf(&mut cx) {
                    break x;
                }
            }
        }};
    }

    struct MaybePending<'a> {
        inner: &'a [u8],
        ready_read: bool,
        ready_fill_buf: bool,
    }

    impl<'a> MaybePending<'a> {
        fn new(inner: &'a [u8]) -> Self {
            Self {
                inner,
                ready_read: false,
                ready_fill_buf: false,
            }
        }
    }

    impl AsyncRead for MaybePending<'_> {
        fn poll_read(
            mut self: Pin<&mut Self>,
            cx: &mut Context<'_>,
            buf: &mut ReadBuf<'_>,
        ) -> Poll<std::io::Result<()>> {
            if self.ready_read {
                self.ready_read = false;
                Pin::new(&mut self.inner).poll_read(cx, buf)
            } else {
                self.ready_read = true;
                cx.waker().wake_by_ref();
                Poll::Pending
            }
        }
    }

    impl AsyncBufRead for MaybePending<'_> {
        fn poll_fill_buf(
            mut self: Pin<&mut Self>,
            _: &mut Context<'_>,
        ) -> Poll<std::io::Result<&[u8]>> {
            if self.ready_fill_buf {
                self.ready_fill_buf = false;
                if self.inner.is_empty() {
                    return Poll::Ready(Ok(&[]));
                }
                let len = cmp::min(2, self.inner.len());
                Poll::Ready(Ok(&self.inner[0..len]))
            } else {
                self.ready_fill_buf = true;
                Poll::Pending
            }
        }

        fn consume(mut self: Pin<&mut Self>, amt: usize) {
            self.inner = &self.inner[amt..];
        }
    }

    impl AsyncSeek for MaybePending<'_> {
        fn start_seek(self: Pin<&mut Self>, pos: SeekFrom) -> std::io::Result<()> {
            assert_eq!(pos, SeekFrom::Start(0));
            Ok(())
        }

        fn poll_complete(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<std::io::Result<u64>> {
            Poll::Ready(Ok(0))
        }
    }

    #[tokio::test]
    async fn test_buffered_reader() {
        let inner: &[u8] = &[5, 6, 7, 0, 1, 2, 3, 4];
        let mut reader = RevBufReader::with_capacity(2, Cursor::new(inner));

        let mut buf = [0, 0, 0];
        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 3);
        assert_eq!(buf, [5, 6, 7]);
        assert!(reader.buffer().is_empty());

        let mut buf = [0, 0];
        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 2);
        assert_eq!(buf, [0, 1]);
        assert!(reader.buffer().is_empty());

        let mut buf = [0];
        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 1);
        assert_eq!(buf, [2]);
        assert_eq!(reader.buffer(), [3]);

        let mut buf = [0, 0, 0];
        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 1);
        assert_eq!(buf, [3, 0, 0]);
        assert!(reader.buffer().is_empty());

        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 1);
        assert_eq!(buf, [4, 0, 0]);
        assert!(reader.buffer().is_empty());

        assert_eq!(reader.read(&mut buf).await.unwrap(), 0);
    }

    #[tokio::test]
    async fn test_buffered_reader_seek() {
        let inner: &[u8] = &[5, 6, 7, 0, 1, 2, 3, 4];
        let mut reader = RevBufReader::with_capacity(2, Cursor::new(inner));

        assert_eq!(reader.seek(SeekFrom::Start(3)).await.unwrap(), 3);
        assert_eq!(run_fill_buf!(reader).unwrap(), &[0, 1][..]);
        assert_eq!(reader.seek(SeekFrom::Start(4)).await.unwrap(), 4);
        assert_eq!(run_fill_buf!(reader).unwrap(), &[1][..]);
        Pin::new(&mut reader).consume(1);
        assert_eq!(reader.seek(SeekFrom::Start(3)).await.unwrap(), 3);
    }

    #[tokio::test]
    async fn test_buffered_reader_seek_underflow() {
        // gimmick reader that yields its position modulo 256 for each byte
        struct PositionReader {
            pos: u64,
        }
        impl AsyncRead for PositionReader {
            fn poll_read(
                mut self: Pin<&mut Self>,
                _: &mut Context<'_>,
                buf: &mut ReadBuf<'_>,
            ) -> Poll<std::io::Result<()>> {
                let b = buf.initialize_unfilled();
                let len = b.len();
                for x in b {
                    *x = self.pos as u8;
                    self.pos = self.pos.wrapping_add(1);
                }
                buf.advance(len);
                Poll::Ready(Ok(()))
            }
        }
        impl AsyncSeek for PositionReader {
            fn start_seek(mut self: Pin<&mut Self>, pos: SeekFrom) -> std::io::Result<()> {
                match pos {
                    SeekFrom::Start(n) => {
                        self.pos = n;
                    }
                    SeekFrom::Current(n) => {
                        self.pos = self.pos.wrapping_add(n as u64);
                    }
                    SeekFrom::End(n) => {
                        self.pos = u64::MAX.wrapping_add(n as u64);
                    }
                }
                Ok(())
            }
            fn poll_complete(
                self: Pin<&mut Self>,
                _: &mut Context<'_>,
            ) -> Poll<std::io::Result<u64>> {
                Poll::Ready(Ok(self.pos))
            }
        }

        let mut reader = RevBufReader::with_capacity(5, PositionReader { pos: 0 });
        assert_eq!(run_fill_buf!(reader).unwrap(), &[0, 1, 2, 3, 4][..]);
        assert_eq!(
            reader.seek(SeekFrom::Start(u64::MAX - 5)).await.unwrap(),
            u64::MAX - 5
        );
        assert_eq!(run_fill_buf!(reader).unwrap().len(), 5);
        // the following seek will require two underlying seeks
        let expected = 9_223_372_036_854_775_802;
        reader.seek(SeekFrom::Start(expected)).await.unwrap();
        assert_eq!(run_fill_buf!(reader).unwrap().len(), 5);
        reader.seek(SeekFrom::Start(expected)).await.unwrap();
        assert_eq!(reader.get_ref().pos, expected + 5);
    }

    #[tokio::test]
    async fn test_short_reads() {
        /// A dummy reader intended at testing short-reads propagation.
        struct ShortReader {
            lengths: Vec<usize>,
        }

        impl AsyncRead for ShortReader {
            fn poll_read(
                mut self: Pin<&mut Self>,
                _: &mut Context<'_>,
                buf: &mut ReadBuf<'_>,
            ) -> Poll<std::io::Result<()>> {
                if !self.lengths.is_empty() {
                    buf.advance(self.lengths.remove(0));
                }
                Poll::Ready(Ok(()))
            }
        }

        impl AsyncSeek for ShortReader {
            fn start_seek(self: Pin<&mut Self>, pos: SeekFrom) -> std::io::Result<()> {
                assert_eq!(pos, SeekFrom::Start(0));
                Ok(())
            }

            fn poll_complete(
                self: Pin<&mut Self>,
                _: &mut Context<'_>,
            ) -> Poll<std::io::Result<u64>> {
                Poll::Ready(Ok(0))
            }
        }

        let inner = ShortReader {
            lengths: vec![0, 1, 2, 0, 1, 0],
        };
        let mut reader = RevBufReader::new(inner);
        let mut buf = [0, 0];
        assert_eq!(reader.read(&mut buf).await.unwrap(), 0);
        assert_eq!(reader.read(&mut buf).await.unwrap(), 1);
        assert_eq!(reader.read(&mut buf).await.unwrap(), 2);
        assert_eq!(reader.read(&mut buf).await.unwrap(), 0);
        assert_eq!(reader.read(&mut buf).await.unwrap(), 1);
        assert_eq!(reader.read(&mut buf).await.unwrap(), 0);
        assert_eq!(reader.read(&mut buf).await.unwrap(), 0);
    }

    #[tokio::test]
    async fn maybe_pending() {
        let inner: &[u8] = &[5, 6, 7, 0, 1, 2, 3, 4];
        let mut reader = RevBufReader::with_capacity(2, MaybePending::new(inner));

        let mut buf = [0, 0, 0];
        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 3);
        assert_eq!(buf, [5, 6, 7]);
        assert!(reader.buffer().is_empty());

        let mut buf = [0, 0];
        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 2);
        assert_eq!(buf, [0, 1]);
        assert!(reader.buffer().is_empty());

        let mut buf = [0];
        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 1);
        assert_eq!(buf, [2]);
        assert_eq!(reader.buffer(), [3]);

        let mut buf = [0, 0, 0];
        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 1);
        assert_eq!(buf, [3, 0, 0]);
        assert!(reader.buffer().is_empty());

        let nread = reader.read(&mut buf).await.unwrap();
        assert_eq!(nread, 1);
        assert_eq!(buf, [4, 0, 0]);
        assert!(reader.buffer().is_empty());

        assert_eq!(reader.read(&mut buf).await.unwrap(), 0);
    }

    #[tokio::test]
    async fn maybe_pending_buf_read() {
        let inner = MaybePending::new(&[0, 1, 2, 3, 1, 0]);
        let mut reader = RevBufReader::with_capacity(2, inner);
        let mut v = Vec::new();
        reader.read_until(3, &mut v).await.unwrap();
        assert_eq!(v, [0, 1, 2, 3]);
        v.clear();
        reader.read_until(1, &mut v).await.unwrap();
        assert_eq!(v, [1]);
        v.clear();
        reader.read_until(8, &mut v).await.unwrap();
        assert_eq!(v, [0]);
        v.clear();
        reader.read_until(9, &mut v).await.unwrap();
        assert!(v.is_empty());
    }

    // https://github.com/rust-lang/futures-rs/pull/1573#discussion_r281162309
    #[tokio::test]
    async fn maybe_pending_seek() {
        struct MaybePendingSeek<'a> {
            inner: Cursor<&'a [u8]>,
            ready: bool,
            seek_res: Option<std::io::Result<()>>,
        }

        impl<'a> MaybePendingSeek<'a> {
            fn new(inner: &'a [u8]) -> Self {
                Self {
                    inner: Cursor::new(inner),
                    ready: true,
                    seek_res: None,
                }
            }
        }

        impl AsyncRead for MaybePendingSeek<'_> {
            fn poll_read(
                mut self: Pin<&mut Self>,
                cx: &mut Context<'_>,
                buf: &mut ReadBuf<'_>,
            ) -> Poll<std::io::Result<()>> {
                Pin::new(&mut self.inner).poll_read(cx, buf)
            }
        }

        impl AsyncBufRead for MaybePendingSeek<'_> {
            fn poll_fill_buf(
                mut self: Pin<&mut Self>,
                cx: &mut Context<'_>,
            ) -> Poll<std::io::Result<&[u8]>> {
                let this: *mut Self = std::ptr::addr_of_mut!(*self);
                Pin::new(&mut unsafe { &mut *this }.inner).poll_fill_buf(cx)
            }

            fn consume(mut self: Pin<&mut Self>, amt: usize) {
                Pin::new(&mut self.inner).consume(amt);
            }
        }

        impl AsyncSeek for MaybePendingSeek<'_> {
            fn start_seek(mut self: Pin<&mut Self>, pos: SeekFrom) -> std::io::Result<()> {
                self.seek_res = Some(Pin::new(&mut self.inner).start_seek(pos));
                Ok(())
            }
            fn poll_complete(
                mut self: Pin<&mut Self>,
                cx: &mut Context<'_>,
            ) -> Poll<std::io::Result<u64>> {
                if self.ready {
                    self.ready = false;
                    self.seek_res.take().unwrap_or(Ok(()))?;
                    Pin::new(&mut self.inner).poll_complete(cx)
                } else {
                    self.ready = true;
                    cx.waker().wake_by_ref();
                    Poll::Pending
                }
            }
        }

        let inner: &[u8] = &[5, 6, 7, 0, 1, 2, 3, 4];
        let mut reader = RevBufReader::with_capacity(2, MaybePendingSeek::new(inner));

        assert_eq!(reader.seek(SeekFrom::Start(3)).await.unwrap(), 3);
        assert_eq!(run_fill_buf!(reader).unwrap(), &[0, 1][..]);
        /*assert!(reader.seek(SeekFrom::Current(i64::MIN)).await.is_err());
        assert_eq!(run_fill_buf!(reader).unwrap(), &[0, 1][..]);
        assert_eq!(reader.seek(SeekFrom::Current(1)).await.unwrap(), 4);
        assert_eq!(run_fill_buf!(reader).unwrap(), &[1, 2][..]);
        Pin::new(&mut reader).consume(1);
        assert_eq!(reader.seek(SeekFrom::Current(-2)).await.unwrap(), 3);*/
    }

    #[macro_export]
    macro_rules! assert_pending {
        ($e:expr) => {{
            use core::task::Poll::*;
            match $e {
                Pending => {}
                Ready(v) => panic!("ready; value = {:?}", v),
            }
        }};
        ($e:expr, $($msg:tt)+) => {{
            use core::task::Poll::*;
            match $e {
                Pending => {}
                Ready(v) => {
                    panic!("ready; value = {:?}; {}", v, format_args!($($msg)+))
                }
            }
        }};
    }

    #[macro_export]
    macro_rules! assert_ready {
        ($e:expr) => {{
            use core::task::Poll::*;
            match $e {
                Ready(v) => v,
                Pending => panic!("pending"),
            }
        }};
        ($e:expr, $($msg:tt)+) => {{
            use core::task::Poll::*;
            match $e {
                Ready(v) => v,
                Pending => {
                    panic!("pending; {}", format_args!($($msg)+))
                }
            }
        }};
    }
}
