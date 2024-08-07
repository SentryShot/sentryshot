#![allow(dead_code)]

#[cfg(test)]
mod test;

use std::io::Write;

use thiserror::Error;

// Mpeg box type.
pub type BoxType = [u8; 4];

//pub type ImmutableBoxes = Vec<dyn ImmutableBox>;

// ImmutableBox is the common trait of boxes.
pub trait ImmutableBox {
    // Type returns the BoxType.
    fn box_type(&self) -> BoxType;

    // Size returns the marshaled size in bytes.
    // The size must be known before marshaling
    // since the box header contains the size.
    fn size(&self) -> usize;

    // Marshal box to writer.
    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error>;
}

#[derive(Debug, Error)]
pub enum Mp4Error {
    #[error("write: {0}")]
    Write(#[from] std::io::Error),

    #[error("from int: {0} {1}")]
    FromInt(String, std::num::TryFromIntError),
}

// Tree of boxes that can be marshaled together.
pub struct Boxes {
    pub mp4_box: Box<dyn ImmutableBox>,
    pub children: Vec<Boxes>,
}

impl Boxes {
    #[must_use]
    pub fn new<T: Into<Box<dyn ImmutableBox>>>(mp4_box: T) -> Self {
        Self {
            mp4_box: mp4_box.into(),
            children: Vec::new(),
        }
    }

    #[must_use]
    pub fn with_child(mut self, child: Boxes) -> Self {
        self.children.push(child);
        self
    }

    #[must_use]
    pub fn with_children2(mut self, child1: Boxes, child2: Boxes) -> Self {
        self.children.extend([child1, child2]);
        self
    }
    #[must_use]

    pub fn with_children3(mut self, child1: Boxes, child2: Boxes, child3: Boxes) -> Self {
        self.children.extend([child1, child2, child3]);
        self
    }

    #[must_use]
    pub fn with_children4(
        mut self,
        child1: Boxes,
        child2: Boxes,
        child3: Boxes,
        child4: Boxes,
    ) -> Self {
        self.children.extend([child1, child2, child3, child4]);
        self
    }

    #[must_use]
    pub fn with_children5(
        mut self,
        child1: Boxes,
        child2: Boxes,
        child3: Boxes,
        child4: Boxes,
        child5: Boxes,
    ) -> Self {
        self.children
            .extend([child1, child2, child3, child4, child5]);
        self
    }

    #[must_use]
    pub fn with_children6(
        mut self,
        child1: Boxes,
        child2: Boxes,
        child3: Boxes,
        child4: Boxes,
        child5: Boxes,
        child6: Boxes,
    ) -> Self {
        self.children
            .extend([child1, child2, child3, child4, child5, child6]);
        self
    }

    #[must_use]
    #[allow(clippy::too_many_arguments)]
    pub fn with_children7(
        mut self,
        child1: Boxes,
        child2: Boxes,
        child3: Boxes,
        child4: Boxes,
        child5: Boxes,
        child6: Boxes,
        child7: Boxes,
    ) -> Self {
        self.children
            .extend([child1, child2, child3, child4, child5, child6, child7]);
        self
    }

    // Size returns the total size of the box including children.
    #[must_use]
    pub fn size(&self) -> usize {
        let mut total = self.mp4_box.size() + 8;

        for child in &self.children {
            let size = child.size();
            total += size;
        }

        total
    }

    // Marshal box including children.
    pub fn marshal<W: Write>(&self, w: &mut W) -> Result<(), Mp4Error> {
        let size = self.size();

        write_box_info(w, size, self.mp4_box.box_type())?;

        // The size of a empty box is 8 bytes.
        if size != 8 {
            self.mp4_box.marshal(w)?;
        }

        for child in &self.children {
            child.marshal(w)?;
        }
        Ok(())
    }
}

pub fn write_box_info<W: Write>(w: &mut W, size: usize, typ: BoxType) -> Result<(), Mp4Error> {
    w.write_all(
        &u32::try_from(size)
            .map_err(|e| Mp4Error::FromInt("write box info".to_owned(), e))?
            .to_be_bytes(),
    )?;
    w.write_all(&typ)?;
    Ok(())
}

pub fn write_single_box<W: Write>(w: &mut W, b: &dyn ImmutableBox) -> Result<usize, Mp4Error> {
    let size = 8 + b.size();

    write_box_info(w, size, b.box_type())?;

    // The size of a empty box is 8 bytes.
    if size != 8 {
        b.marshal(w)?;
    }
    Ok(size)
}

/*
// Marshal ImmutableBoxes to writer.
fn (boxes ImmutableBoxes) marshal(&self, w: &mut dyn std::io::Write) -> Result<(), MarshalError> {
    for _, b := range boxes {
        if _, err := WriteSingleBox(w, b); err != nil {
            return err
        }
    }
    return nil
}

// Size combined size of boxes.
fn (boxes ImmutableBoxes) size(&self) -> usize {
    var n int
    for _, b := range boxes {
        n += 8
        n += b.Size()
    }
    return n
}

*/
/************************* FullBox **************************/

#[derive(Clone, Copy, Default)]
pub struct FullBox {
    pub version: u8,
    pub flags: [u8; 3],
}

impl FullBox {
    fn get_flags(self) -> u32 {
        parse_fullbox_flags(self.flags)
    }

    fn check_flag(self, flag: u32) -> bool {
        self.get_flags() & flag != 0
    }

    pub fn marshal_field(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        w.write_all(&[self.version])?;
        w.write_all(&self.flags)?;
        Ok(())
    }
}

fn parse_fullbox_flags(flags: [u8; 3]) -> u32 {
    (u32::from(flags[0]) << 16) ^ (u32::from(flags[1]) << 8) ^ (u32::from(flags[2]))
}

fn check_fullbox_flag(flags: [u8; 3], flag: u32) -> bool {
    let flags = parse_fullbox_flags(flags);
    flags & flag != 0
}

#[must_use]
#[allow(clippy::cast_possible_truncation, clippy::as_conversions)]
pub fn u32_to_flags(v: u32) -> [u8; 3] {
    [(v >> 16) as u8, (v >> 8) as u8, v as u8]
}

/*************************** btrt ****************************/

pub const TYPE_BTRT: BoxType = *b"btrt";

pub struct Btrt {
    pub buffer_size_db: u32,
    pub max_bitrate: u32,
    pub avg_bitrate: u32,
}

impl ImmutableBox for Btrt {
    fn box_type(&self) -> BoxType {
        TYPE_BTRT
    }

    fn size(&self) -> usize {
        12
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        w.write_all(&self.buffer_size_db.to_be_bytes())?;
        w.write_all(&self.max_bitrate.to_be_bytes())?;
        w.write_all(&self.avg_bitrate.to_be_bytes())?;
        Ok(())
    }
}

impl From<Btrt> for Box<dyn ImmutableBox> {
    fn from(value: Btrt) -> Self {
        Box::new(value)
    }
}

/*************************** ctts ****************************/

pub const TYPE_CTTS: BoxType = *b"ctts";

pub struct Ctts {
    pub flags: [u8; 3],
    pub entries: CttsEntries,
}

pub enum CttsEntries {
    V0(Vec<CttsEntryV0>),
    V1(Vec<CttsEntryV1>),
}

#[derive(Clone, Copy)]
pub struct CttsEntryV0 {
    pub sample_count: u32,
    pub sample_offset: u32,
}

#[derive(Clone, Copy)]
pub struct CttsEntryV1 {
    pub sample_count: u32,
    pub sample_offset: i32,
}

impl ImmutableBox for Ctts {
    fn box_type(&self) -> BoxType {
        TYPE_CTTS
    }

    fn size(&self) -> usize {
        let num_entries = match &self.entries {
            CttsEntries::V0(v) => v.len(),
            CttsEntries::V1(v) => v.len(),
        };
        8 + num_entries * 8
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        match &self.entries {
            CttsEntries::V0(entries) => {
                w.write_all(&[0])?;
                w.write_all(&self.flags)?;

                w.write_all(
                    &(u32::try_from(entries.len())
                        .map_err(|e| Mp4Error::FromInt("ctts".to_owned(), e))?)
                    .to_be_bytes(),
                )?;
                for entry in entries {
                    w.write_all(&entry.sample_count.to_be_bytes())?;
                    w.write_all(&entry.sample_offset.to_be_bytes())?;
                }
            }
            CttsEntries::V1(entries) => {
                w.write_all(&[1])?;
                w.write_all(&self.flags)?;

                w.write_all(
                    &(u32::try_from(entries.len())
                        .map_err(|e| Mp4Error::FromInt("ctts".to_owned(), e))?)
                    .to_be_bytes(),
                )?;
                for entry in entries {
                    w.write_all(&entry.sample_count.to_be_bytes())?;
                    w.write_all(&entry.sample_offset.to_be_bytes())?;
                }
            }
        }

        Ok(())
    }
}

impl From<Ctts> for Box<dyn ImmutableBox> {
    fn from(value: Ctts) -> Self {
        Box::new(value)
    }
}

/*************************** dinf ****************************/

pub const TYPE_DINF: BoxType = *b"dinf";

pub struct Dinf;

impl ImmutableBox for Dinf {
    fn box_type(&self) -> BoxType {
        TYPE_DINF
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Dinf> for Box<dyn ImmutableBox> {
    fn from(value: Dinf) -> Self {
        Box::new(value)
    }
}

/*************************** dref ****************************/

pub const TYPE_DREF: BoxType = *b"dref";

pub struct Dref {
    pub full_box: FullBox,
    pub entry_count: u32,
}

impl ImmutableBox for Dref {
    fn box_type(&self) -> BoxType {
        TYPE_DREF
    }

    fn size(&self) -> usize {
        8
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(&self.entry_count.to_be_bytes())?;
        Ok(())
    }
}

impl From<Dref> for Box<dyn ImmutableBox> {
    fn from(value: Dref) -> Self {
        Box::new(value)
    }
}

/*************************** url ****************************/

pub const TYPE_URL: BoxType = *b"url ";

pub struct Url {
    pub full_box: FullBox,
    pub location: String,
}

pub const URL_NOPT: u32 = 0x0000_0001;

impl ImmutableBox for Url {
    fn box_type(&self) -> BoxType {
        TYPE_URL
    }

    fn size(&self) -> usize {
        if self.full_box.check_flag(URL_NOPT) {
            4
        } else {
            self.location.len() + 5
        }
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        if !self.full_box.check_flag(URL_NOPT) {
            w.write_all((self.location.clone() + "\0").as_bytes())?;
        }
        Ok(())
    }
}

impl From<Url> for Box<dyn ImmutableBox> {
    fn from(value: Url) -> Self {
        Box::new(value)
    }
}

/*************************** edts ****************************/

pub const TYPE_EDTS: BoxType = *b"edts";

pub struct Edts;

impl ImmutableBox for Edts {
    fn box_type(&self) -> BoxType {
        TYPE_EDTS
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Edts> for Box<dyn ImmutableBox> {
    fn from(value: Edts) -> Self {
        Box::new(value)
    }
}

/*************************** elst ****************************/

pub const TYPE_ELST: BoxType = *b"elst";

#[derive(Clone)]
pub struct Elst {
    pub flags: [u8; 3],
    pub entries: ElstEntries,
}

impl ImmutableBox for Elst {
    fn box_type(&self) -> BoxType {
        TYPE_ELST
    }

    fn size(&self) -> usize {
        match &self.entries {
            ElstEntries::V0(v) => 8 + v.len() * 12,
            ElstEntries::V1(v) => 8 + v.len() * 20,
        }
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        match &self.entries {
            ElstEntries::V0(entries) => {
                w.write_all(&[0])?;
                w.write_all(&self.flags)?;
                w.write_all(
                    &u32::try_from(entries.len())
                        .map_err(|e| Mp4Error::FromInt("elst".to_owned(), e))?
                        .to_be_bytes(),
                )?;
                for entry in entries {
                    w.write_all(&entry.segment_duration.to_be_bytes())?;
                    w.write_all(&entry.media_time.to_be_bytes())?;
                    w.write_all(&entry.media_rate_integer.to_be_bytes())?;
                    w.write_all(&entry.media_rate_fraction.to_be_bytes())?;
                }
            }
            ElstEntries::V1(entries) => {
                w.write_all(&[1])?;
                w.write_all(&self.flags)?;
                w.write_all(
                    &u32::try_from(entries.len())
                        .map_err(|e| Mp4Error::FromInt("elst".to_owned(), e))?
                        .to_be_bytes(),
                )?;
                for entry in entries {
                    w.write_all(&entry.segment_duration.to_be_bytes())?;
                    w.write_all(&entry.media_time.to_be_bytes())?;
                    w.write_all(&entry.media_rate_integer.to_be_bytes())?;
                    w.write_all(&entry.media_rate_fraction.to_be_bytes())?;
                }
            }
        }
        Ok(())
    }
}

#[derive(Clone)]
pub enum ElstEntries {
    V0(Vec<ElstEntryV0>),
    V1(Vec<ElstEntryV1>),
}

#[derive(Clone)]
pub struct ElstEntryV0 {
    pub segment_duration: u32,
    pub media_time: i32,
    pub media_rate_integer: i16,
    pub media_rate_fraction: i16,
}

impl Default for ElstEntryV0 {
    fn default() -> Self {
        Self {
            segment_duration: 0,
            media_time: 0,
            media_rate_integer: 1,
            media_rate_fraction: 0,
        }
    }
}

#[derive(Clone)]
pub struct ElstEntryV1 {
    pub segment_duration: u64,
    pub media_time: i64,
    pub media_rate_integer: i16,
    pub media_rate_fraction: i16,
}

impl Default for ElstEntryV1 {
    fn default() -> Self {
        Self {
            segment_duration: 0,
            media_time: 0,
            media_rate_integer: 1,
            media_rate_fraction: 0,
        }
    }
}

impl From<Elst> for Box<dyn ImmutableBox> {
    fn from(value: Elst) -> Self {
        Box::new(value)
    }
}

/*************************** ftyp ****************************/

pub const TYPE_FTYP: BoxType = *b"ftyp";

pub struct Ftyp {
    pub major_brand: [u8; 4],
    pub minor_version: u32,
    pub compatible_brands: Vec<CompatibleBrandElem>,
}

#[repr(transparent)]
pub struct CompatibleBrandElem(pub [u8; 4]);

impl ImmutableBox for Ftyp {
    fn box_type(&self) -> BoxType {
        TYPE_FTYP
    }

    fn size(&self) -> usize {
        8 + self.compatible_brands.len() * 4
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        w.write_all(&self.major_brand)?;
        w.write_all(&self.minor_version.to_be_bytes())?;
        for brands in &self.compatible_brands {
            w.write_all(&brands.0)?;
        }
        Ok(())
    }
}

impl From<Ftyp> for Box<dyn ImmutableBox> {
    fn from(value: Ftyp) -> Self {
        Box::new(value)
    }
}

/*************************** hdlr ****************************/

pub const TYPE_HDLR: BoxType = *b"hdlr";

#[derive(Default)]
pub struct Hdlr {
    pub full_box: FullBox,
    // Predefined corresponds to component_type of QuickTime.
    // pre_defined of ISO-14496 has albufays zero,
    // hobufever component_type has "mhlr" or "dhlr".
    pub pre_defined: u32,
    pub handler_type: [u8; 4],
    pub reserved: [u32; 3],
    pub name: String,
}

impl ImmutableBox for Hdlr {
    fn box_type(&self) -> BoxType {
        TYPE_HDLR
    }

    fn size(&self) -> usize {
        25 + self.name.len()
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(&self.pre_defined.to_be_bytes())?;
        w.write_all(&self.handler_type)?;
        for reserved in &self.reserved {
            w.write_all(&reserved.to_be_bytes())?;
        }
        w.write_all((self.name.clone() + "\0").as_bytes())?;
        Ok(())
    }
}

impl From<Hdlr> for Box<dyn ImmutableBox> {
    fn from(value: Hdlr) -> Self {
        Box::new(value)
    }
}

/*************************** mdat ****************************/

pub const TYPE_MDAT: BoxType = *b"mdat";

pub struct Mdat(pub Vec<u8>);

impl ImmutableBox for Mdat {
    fn box_type(&self) -> BoxType {
        TYPE_MDAT
    }

    fn size(&self) -> usize {
        self.0.len()
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        w.write_all(&self.0)?;
        Ok(())
    }
}

impl From<Mdat> for Box<dyn ImmutableBox> {
    fn from(value: Mdat) -> Self {
        Box::new(value)
    }
}

/*************************** mdia ****************************/

pub const TYPE_MDIA: BoxType = *b"mdia";

pub struct Mdia;

impl ImmutableBox for Mdia {
    fn box_type(&self) -> BoxType {
        TYPE_MDIA
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Mdia> for Box<dyn ImmutableBox> {
    fn from(value: Mdia) -> Self {
        Box::new(value)
    }
}

/*************************** mdhd ****************************/

pub const TYPE_MDHD: BoxType = *b"mdhd";

#[derive(Default)]
pub struct Mdhd {
    pub flags: [u8; 3],
    pub version: MdhdVersion,
    pub timescale: u32,

    pub pad: bool,         // 1 bit.
    pub language: [u8; 3], // 5 bits. ISO-639-2/T language code
    pub pre_defined: u16,
}

pub enum MdhdVersion {
    V0(MdhdV0),
    V1(MdhdV1),
}

impl Default for MdhdVersion {
    fn default() -> Self {
        Self::V0(MdhdV0::default())
    }
}

#[derive(Default)]
pub struct MdhdV0 {
    pub creation_time: u32,
    pub modification_time: u32,
    pub duration: u32,
}

pub struct MdhdV1 {
    pub creation_time: u64,
    pub modification_time: u64,
    pub duration: u64,
}

impl ImmutableBox for Mdhd {
    fn box_type(&self) -> BoxType {
        TYPE_MDHD
    }

    fn size(&self) -> usize {
        match self.version {
            MdhdVersion::V0(_) => 24,
            MdhdVersion::V1(_) => 36,
        }
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        match &self.version {
            MdhdVersion::V0(v) => {
                w.write_all(&[0])?;
                w.write_all(&self.flags)?;
                w.write_all(&v.creation_time.to_be_bytes())?;
                w.write_all(&v.modification_time.to_be_bytes())?;
                w.write_all(&self.timescale.to_be_bytes())?;
                w.write_all(&v.duration.to_be_bytes())?;
            }
            MdhdVersion::V1(v) => {
                w.write_all(&[1])?;
                w.write_all(&self.flags)?;
                w.write_all(&v.creation_time.to_be_bytes())?;
                w.write_all(&v.modification_time.to_be_bytes())?;
                w.write_all(&self.timescale.to_be_bytes())?;
                w.write_all(&v.duration.to_be_bytes())?;
            }
        }

        if self.pad {
            w.write_all(&[(0b0000_0001 << 7
                | (self.language[0] & 0b0001_1111) << 2
                | (self.language[1] & 0b0001_1111) >> 3)])?;
        } else {
            w.write_all(&[
                ((self.language[0] & 0b0001_1111) << 2 | (self.language[1] & 0b0001_1111) >> 3)
            ])?;
        }

        w.write_all(&[(self.language[1] << 5 | self.language[2] & 0b0001_1111)])?;
        w.write_all(&self.pre_defined.to_be_bytes())?;
        Ok(())
    }
}

impl From<Mdhd> for Box<dyn ImmutableBox> {
    fn from(value: Mdhd) -> Self {
        Box::new(value)
    }
}

/*************************** mfhd ****************************/

pub const TYPE_MFHD: BoxType = *b"mfhd";

pub struct Mfhd {
    pub full_box: FullBox,
    pub sequence_number: u32,
}

impl ImmutableBox for Mfhd {
    fn box_type(&self) -> BoxType {
        TYPE_MFHD
    }

    fn size(&self) -> usize {
        8
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(&self.sequence_number.to_be_bytes())?;
        Ok(())
    }
}

impl From<Mfhd> for Box<dyn ImmutableBox> {
    fn from(value: Mfhd) -> Self {
        Box::new(value)
    }
}

/*************************** minf ****************************/

pub const TYPE_MINF: BoxType = *b"minf";

pub struct Minf;

impl ImmutableBox for Minf {
    fn box_type(&self) -> BoxType {
        TYPE_MINF
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Minf> for Box<dyn ImmutableBox> {
    fn from(value: Minf) -> Self {
        Box::new(value)
    }
}

/*************************** moof ****************************/

pub const TYPE_MOOF: BoxType = *b"moof";

pub struct Moof;

impl ImmutableBox for Moof {
    fn box_type(&self) -> BoxType {
        TYPE_MOOF
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Moof> for Box<dyn ImmutableBox> {
    fn from(value: Moof) -> Self {
        Box::new(value)
    }
}

/*************************** moov ****************************/

pub const TYPE_MOOV: BoxType = *b"moov";

pub struct Moov;

impl ImmutableBox for Moov {
    fn box_type(&self) -> BoxType {
        TYPE_MOOV
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Moov> for Box<dyn ImmutableBox> {
    fn from(value: Moov) -> Self {
        Box::new(value)
    }
}

/*************************** mvex ****************************/

pub const TYPE_MVEX: BoxType = *b"mvex";

pub struct Mvex;

impl ImmutableBox for Mvex {
    fn box_type(&self) -> BoxType {
        TYPE_MVEX
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Mvex> for Box<dyn ImmutableBox> {
    fn from(value: Mvex) -> Self {
        Box::new(value)
    }
}

/*************************** mvhd ****************************/

pub const TYPE_MVHD: BoxType = *b"mvhd";

#[derive(Default)]
pub struct Mvhd {
    pub flags: [u8; 3],
    pub version: MvhdVersion,
    pub timescale: u32,
    pub rate: i32,   // fixed-point 16.16 - template=0x00010000
    pub volume: i16, // template=0x0100
    pub reserved: i16,
    pub reserved2: [i32; 2],
    pub matrix: [i32; 9], // template={ 0x00010000,0,0,0,0x00010000,0,0,0,0x40000000 }
    pub pre_defined: [i32; 6],
    pub next_track_id: u32,
}

pub enum MvhdVersion {
    V0(MvhdV0),
    V1(MvhdV1),
}

impl Default for MvhdVersion {
    fn default() -> Self {
        Self::V0(MvhdV0::default())
    }
}

#[derive(Default)]
pub struct MvhdV0 {
    pub creation_time: u32,
    pub modification_time: u32,
    pub duration: u32,
}

pub struct MvhdV1 {
    pub creation_time: u64,
    pub modification_time: u64,
    pub duration: u64,
}

impl ImmutableBox for Mvhd {
    fn box_type(&self) -> BoxType {
        TYPE_MVHD
    }

    fn size(&self) -> usize {
        match self.version {
            MvhdVersion::V0(_) => 100,
            MvhdVersion::V1(_) => 112,
        }
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        match &self.version {
            MvhdVersion::V0(v) => {
                w.write_all(&[0])?;
                w.write_all(&self.flags)?;
                w.write_all(&v.creation_time.to_be_bytes())?;
                w.write_all(&v.modification_time.to_be_bytes())?;
                w.write_all(&self.timescale.to_be_bytes())?;
                w.write_all(&v.duration.to_be_bytes())?;
            }
            MvhdVersion::V1(v) => {
                w.write_all(&[1])?;
                w.write_all(&self.flags)?;
                w.write_all(&v.creation_time.to_be_bytes())?;
                w.write_all(&v.modification_time.to_be_bytes())?;
                w.write_all(&self.timescale.to_be_bytes())?;
                w.write_all(&v.duration.to_be_bytes())?;
            }
        }

        w.write_all(&self.rate.to_be_bytes())?;
        w.write_all(&self.volume.to_be_bytes())?;
        w.write_all(&self.reserved.to_be_bytes())?;

        for reserved in &self.reserved2 {
            w.write_all(&reserved.to_be_bytes())?;
        }
        for matrix in &self.matrix {
            w.write_all(&matrix.to_be_bytes())?;
        }
        for pre_defined in &self.pre_defined {
            w.write_all(&pre_defined.to_be_bytes())?;
        }

        w.write_all(&self.next_track_id.to_be_bytes())?;

        Ok(())
    }
}

impl From<Mvhd> for Box<dyn ImmutableBox> {
    fn from(value: Mvhd) -> Self {
        Box::new(value)
    }
}

/*********************** SampleEntry *************************/

#[derive(Default)]
pub struct SampleEntry {
    pub reserved: [u8; 6],
    pub data_reference_index: u16,
}

impl SampleEntry {
    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        for reserved in &self.reserved {
            w.write_all(&reserved.to_be_bytes())?;
        }
        w.write_all(&self.data_reference_index.to_be_bytes())?;
        Ok(())
    }
}

/*********************** avc1 *************************/

pub const TYPE_AVC1: BoxType = *b"avc1";

#[derive(Default)]
pub struct Avc1 {
    pub sample_entry: SampleEntry,
    pub pre_defined: u16,
    pub reserved: u16,
    pub pre_defined2: [u32; 3],
    pub width: u16,
    pub height: u16,
    pub horiz_resolution: u32,
    pub vert_resolution: u32,
    pub reserved2: u32,
    pub frame_count: u16,
    pub compressor_name: [u8; 32],
    pub depth: u16,
    pub pre_defined3: i16,
}

impl ImmutableBox for Avc1 {
    fn box_type(&self) -> BoxType {
        TYPE_AVC1
    }

    fn size(&self) -> usize {
        78
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.sample_entry.marshal(w)?;
        w.write_all(&self.pre_defined.to_be_bytes())?;
        w.write_all(&self.reserved.to_be_bytes())?;
        for pre_defined in &self.pre_defined2 {
            w.write_all(&pre_defined.to_be_bytes())?;
        }
        w.write_all(&self.width.to_be_bytes())?;
        w.write_all(&self.height.to_be_bytes())?;
        w.write_all(&self.horiz_resolution.to_be_bytes())?;
        w.write_all(&self.vert_resolution.to_be_bytes())?;
        w.write_all(&self.reserved2.to_be_bytes())?;
        w.write_all(&self.frame_count.to_be_bytes())?;
        w.write_all(&self.compressor_name)?;
        w.write_all(&self.depth.to_be_bytes())?;
        w.write_all(&self.pre_defined3.to_be_bytes())?;
        Ok(())
    }
}

impl From<Avc1> for Box<dyn ImmutableBox> {
    fn from(value: Avc1) -> Self {
        Box::new(value)
    }
}

/**************** AVCDecoderConfiguration ****************.*/
pub const AVC_BASELINE_PROFILE: u8 = 66; // 0x42
pub const AVC_MAIN_PROFILE: u8 = 77; // 0x4d
pub const AVC_EXTENDED_PROFILE: u8 = 88; // 0x58
pub const AVC_HIGH_PROFILE: u8 = 100; // 0x64
pub const AVC_HIGH_10_PROFILE: u8 = 110; // 0x6e
pub const AVC_HIGH_422_PROFILE: u8 = 122; // 0x7a

pub struct AvcParameterSet(Vec<u8>);

impl AvcParameterSet {
    fn field_size(&self) -> usize {
        self.0.len() + 2
    }

    fn marshal_field(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        w.write_all(
            &u16::try_from(self.0.len())
                .map_err(|e| Mp4Error::FromInt("parameter set".to_owned(), e))?
                .to_be_bytes(),
        )?;
        w.write_all(&self.0)?;
        Ok(())
    }
}

/*************************** avcC ****************************/

pub const TYPE_AVCC: BoxType = *b"avcC";

pub struct AvcC {
    pub configuration_version: u8,
    pub profile: u8,
    pub profile_compatibility: u8,
    pub level: u8,
    pub reserved: u8,                       // 6 bits.
    pub length_size_minus_one: u8,          // 2 bits.
    pub reserved2: u8,                      // 3 bits.
    pub num_of_sequence_parameter_sets: u8, // 5 bits.
    pub sequence_parameter_sets: Vec<AvcParameterSet>,
    pub num_of_picture_parameter_sets: u8,
    pub picture_parameter_sets: Vec<AvcParameterSet>,
    pub high_profile_fields_enabled: bool,
    pub reserved3: u8,               // 6 bits.
    pub chroma_format: u8,           // 2 bits.
    pub reserved4: u8,               // 5 bits.
    pub bitdepth_luma_minus_8: u8,   // 3 bits.
    pub reserved5: u8,               // 5 bits.
    pub bitdepth_chroma_minus_8: u8, // 3 bits.
    pub num_of_sequence_parameter_set_ext: u8,
    pub sequence_parameter_sets_ext: Vec<AvcParameterSet>,
}

impl ImmutableBox for AvcC {
    fn box_type(&self) -> BoxType {
        TYPE_AVCC
    }

    fn size(&self) -> usize {
        let mut total = 7;
        for sets in &self.sequence_parameter_sets {
            total += sets.field_size();
        }
        for sets in &self.picture_parameter_sets {
            total += sets.field_size();
        }
        if self.reserved3 != 0 {
            total += 4;
            for sets in &self.sequence_parameter_sets_ext {
                total += sets.field_size();
            }
        }
        total
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        w.write_all(&self.configuration_version.to_be_bytes())?;
        w.write_all(&self.profile.to_be_bytes())?;
        w.write_all(&self.profile_compatibility.to_be_bytes())?;
        w.write_all(&self.level.to_be_bytes())?;
        w.write_all(&[self.reserved << 2 | self.length_size_minus_one & 0b0000_0011])?;
        w.write_all(&[self.reserved2 << 5 | self.num_of_sequence_parameter_sets & 0b0001_1111])?;
        for sets in &self.sequence_parameter_sets {
            sets.marshal_field(w)?;
        }
        w.write_all(&self.num_of_picture_parameter_sets.to_be_bytes())?;
        for sets in &self.picture_parameter_sets {
            sets.marshal_field(w)?;
        }
        if self.high_profile_fields_enabled
            && self.profile != AVC_HIGH_PROFILE
            && self.profile != AVC_HIGH_10_PROFILE
            && self.profile != AVC_HIGH_422_PROFILE
            && self.profile != 144
        {
            panic!("fmp4 each values of profile and high_profile_fields_enabled are inconsistent")
        }
        if self.reserved3 != 0 {
            w.write_all(&[self.reserved3 << 2 | self.chroma_format & 0b0000_0011])?;
            w.write_all(&[self.reserved4 << 3 | self.bitdepth_luma_minus_8 & 0b0000_0111])?;
            w.write_all(&[self.reserved5 << 3 | self.bitdepth_chroma_minus_8 & 0b0000_0111])?;
            w.write_all(&self.num_of_sequence_parameter_set_ext.to_be_bytes())?;
            for sets in &self.sequence_parameter_sets_ext {
                sets.marshal_field(w)?;
            }
        }
        Ok(())
    }
}

impl From<AvcC> for Box<dyn ImmutableBox> {
    fn from(value: AvcC) -> Self {
        Box::new(value)
    }
}

/*************************** stbl ****************************/

pub const TYPE_STBL: BoxType = *b"stbl";

pub struct Stbl;

impl ImmutableBox for Stbl {
    fn box_type(&self) -> BoxType {
        TYPE_STBL
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Stbl> for Box<dyn ImmutableBox> {
    fn from(value: Stbl) -> Self {
        Box::new(value)
    }
}

/*************************** stco ****************************/

pub const TYPE_STCO: BoxType = *b"stco";

#[derive(Default)]
pub struct Stco {
    pub full_box: FullBox,
    pub chunk_offsets: Vec<u32>,
}

impl ImmutableBox for Stco {
    fn box_type(&self) -> BoxType {
        TYPE_STCO
    }

    fn size(&self) -> usize {
        8 + (self.chunk_offsets.len()) * 4
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(
            &u32::try_from(self.chunk_offsets.len())
                .map_err(|e| Mp4Error::FromInt("stco".to_owned(), e))?
                .to_be_bytes(),
        )?;
        for offset in &self.chunk_offsets {
            w.write_all(&offset.to_be_bytes())?;
        }
        Ok(())
    }
}

impl From<Stco> for Box<dyn ImmutableBox> {
    fn from(value: Stco) -> Self {
        Box::new(value)
    }
}

/*************************** stsc ****************************/

pub const TYPE_STSC: BoxType = *b"stsc";

#[derive(Clone, Copy)]
pub struct StscEntry {
    pub first_chunk: u32,
    pub samples_per_chunk: u32,
    pub sample_description_index: u32,
}

impl StscEntry {
    fn marshal_field(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        w.write_all(&self.first_chunk.to_be_bytes())?;
        w.write_all(&self.samples_per_chunk.to_be_bytes())?;
        w.write_all(&self.sample_description_index.to_be_bytes())?;
        Ok(())
    }
}

#[derive(Default)]
pub struct Stsc {
    pub full_box: FullBox,
    pub entries: Vec<StscEntry>,
}

impl ImmutableBox for Stsc {
    fn box_type(&self) -> BoxType {
        TYPE_STSC
    }

    fn size(&self) -> usize {
        8 + self.entries.len() * 12
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(
            &u32::try_from(self.entries.len())
                .map_err(|e| Mp4Error::FromInt("stsc".to_owned(), e))?
                .to_be_bytes(),
        )?; // Entry count.
        for entry in &self.entries {
            entry.marshal_field(w)?;
        }
        Ok(())
    }
}

impl From<Stsc> for Box<dyn ImmutableBox> {
    fn from(value: Stsc) -> Self {
        Box::new(value)
    }
}

/*************************** stsd ****************************/

pub const TYPE_STSD: BoxType = *b"stsd";

pub struct Stsd {
    pub full_box: FullBox,
    pub entry_count: u32,
}

impl ImmutableBox for Stsd {
    fn box_type(&self) -> BoxType {
        TYPE_STSD
    }

    fn size(&self) -> usize {
        8
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(&self.entry_count.to_be_bytes())?;
        Ok(())
    }
}

impl From<Stsd> for Box<dyn ImmutableBox> {
    fn from(value: Stsd) -> Self {
        Box::new(value)
    }
}

/*************************** stss ****************************/

pub const TYPE_STSS: BoxType = *b"stss";

pub struct Stss {
    pub full_box: FullBox,
    pub sample_numbers: Vec<u32>,
}

impl ImmutableBox for Stss {
    fn box_type(&self) -> BoxType {
        TYPE_STSS
    }

    fn size(&self) -> usize {
        8 + self.sample_numbers.len() * 4
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(
            &u32::try_from(self.sample_numbers.len())
                .map_err(|e| Mp4Error::FromInt("stss".to_owned(), e))?
                .to_be_bytes(),
        )?; // Entry count.
        for number in &self.sample_numbers {
            w.write_all(&number.to_be_bytes())?;
        }
        Ok(())
    }
}

impl From<Stss> for Box<dyn ImmutableBox> {
    fn from(value: Stss) -> Self {
        Box::new(value)
    }
}

/*************************** stsz ****************************/

pub const TYPE_STSZ: BoxType = *b"stsz";

#[derive(Default)]
pub struct Stsz {
    pub full_box: FullBox,
    pub sample_size: u32,
    pub sample_count: u32,
    pub entry_sizes: Vec<u32>,
}

impl ImmutableBox for Stsz {
    fn box_type(&self) -> BoxType {
        TYPE_STSZ
    }

    fn size(&self) -> usize {
        12 + self.entry_sizes.len() * 4
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(&self.sample_size.to_be_bytes())?;
        w.write_all(&self.sample_count.to_be_bytes())?;
        for entry in &self.entry_sizes {
            w.write_all(&entry.to_be_bytes())?;
        }
        Ok(())
    }
}

impl From<Stsz> for Box<dyn ImmutableBox> {
    fn from(value: Stsz) -> Self {
        Box::new(value)
    }
}

/*************************** stts ****************************/

pub const TYPE_STTS: BoxType = *b"stts";

#[derive(Default)]
pub struct Stts {
    pub full_box: FullBox,
    pub entries: Vec<SttsEntry>,
}

#[derive(Clone)]
pub struct SttsEntry {
    pub sample_count: u32,
    pub sample_delta: u32,
}

impl SttsEntry {
    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        w.write_all(&self.sample_count.to_be_bytes())?;
        w.write_all(&self.sample_delta.to_be_bytes())?;
        Ok(())
    }
}

impl ImmutableBox for Stts {
    fn box_type(&self) -> BoxType {
        TYPE_STTS
    }

    fn size(&self) -> usize {
        8 + self.entries.len() * 8
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(
            &u32::try_from(self.entries.len())
                .map_err(|e| Mp4Error::FromInt("stts".to_owned(), e))?
                .to_be_bytes(),
        )?;
        for entry in &self.entries {
            entry.marshal(w)?;
        }
        Ok(())
    }
}

impl From<Stts> for Box<dyn ImmutableBox> {
    fn from(value: Stts) -> Self {
        Box::new(value)
    }
}

/*************************** tfdt ****************************/

pub const TYPE_TFDT: BoxType = *b"tfdt";

pub struct Tfdt {
    pub flags: [u8; 3],
    pub base_media_decode_time: TfdtBaseMediaDecodeTime,
}

pub enum TfdtBaseMediaDecodeTime {
    V0(u32),
    V1(u64),
}

impl ImmutableBox for Tfdt {
    fn box_type(&self) -> BoxType {
        TYPE_TFDT
    }

    fn size(&self) -> usize {
        match self.base_media_decode_time {
            TfdtBaseMediaDecodeTime::V0(_) => 8,
            TfdtBaseMediaDecodeTime::V1(_) => 12,
        }
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        match self.base_media_decode_time {
            TfdtBaseMediaDecodeTime::V0(v) => {
                w.write_all(&[0])?;
                w.write_all(&self.flags)?;
                w.write_all(&v.to_be_bytes())?;
            }
            TfdtBaseMediaDecodeTime::V1(v) => {
                w.write_all(&[1])?;
                w.write_all(&self.flags)?;
                w.write_all(&v.to_be_bytes())?;
            }
        }
        Ok(())
    }
}

impl From<Tfdt> for Box<dyn ImmutableBox> {
    fn from(value: Tfdt) -> Self {
        Box::new(value)
    }
}

/*************************** tfhd ****************************/

pub const TYPE_TFHD: BoxType = *b"tfhd";

#[derive(Default)]
pub struct Tfhd {
    pub full_box: FullBox,
    pub track_id: u32,

    // optional
    pub base_data_offset: u64,
    pub sample_descroption_index: u32,
    pub default_sample_duration: u32,
    pub default_sample_size: u32,
    pub default_sample_flags: u32,
}

pub const TFHD_BASE_DATA_OFFSET_PRESENT: u32 = 0x0000_0001;
pub const TFHD_SAMPLE_DESCRIPTION_INDEX_PRESENT: u32 = 0x0000_0002;
pub const TFHD_DEFAULT_SAMPLE_DURATION_PRESENT: u32 = 0x0000_0008;
pub const TFHD_DEFAULT_SAMPLE_SIZE_PRESENT: u32 = 0x0000_0010;
pub const TFHD_DEFAULT_SAMPLE_FLAGS_PRESENT: u32 = 0x0000_0020;

impl ImmutableBox for Tfhd {
    fn box_type(&self) -> BoxType {
        TYPE_TFHD
    }

    fn size(&self) -> usize {
        let mut total: usize = 8;
        if self.full_box.check_flag(TFHD_BASE_DATA_OFFSET_PRESENT) {
            total += 8;
        }
        if self
            .full_box
            .check_flag(TFHD_SAMPLE_DESCRIPTION_INDEX_PRESENT)
        {
            total += 4;
        }
        if self
            .full_box
            .check_flag(TFHD_DEFAULT_SAMPLE_DURATION_PRESENT)
        {
            total += 4;
        }
        if self.full_box.check_flag(TFHD_DEFAULT_SAMPLE_SIZE_PRESENT) {
            total += 4;
        }
        if self.full_box.check_flag(TFHD_DEFAULT_SAMPLE_FLAGS_PRESENT) {
            total += 4;
        }
        total
    }

    // Marshal box to writer.
    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(&self.track_id.to_be_bytes())?;
        if self.full_box.check_flag(TFHD_BASE_DATA_OFFSET_PRESENT) {
            w.write_all(&self.base_data_offset.to_be_bytes())?;
        }
        if self
            .full_box
            .check_flag(TFHD_SAMPLE_DESCRIPTION_INDEX_PRESENT)
        {
            w.write_all(&self.sample_descroption_index.to_be_bytes())?;
        }
        if self
            .full_box
            .check_flag(TFHD_DEFAULT_SAMPLE_DURATION_PRESENT)
        {
            w.write_all(&self.default_sample_duration.to_be_bytes())?;
        }
        if self.full_box.check_flag(TFHD_DEFAULT_SAMPLE_SIZE_PRESENT) {
            w.write_all(&self.default_sample_size.to_be_bytes())?;
        }
        if self.full_box.check_flag(TFHD_DEFAULT_SAMPLE_FLAGS_PRESENT) {
            w.write_all(&self.default_sample_flags.to_be_bytes())?;
        }
        Ok(())
    }
}

impl From<Tfhd> for Box<dyn ImmutableBox> {
    fn from(value: Tfhd) -> Self {
        Box::new(value)
    }
}

/*************************** tkhd ****************************/

pub const TYPE_TKHD: BoxType = *b"tkhd";

#[derive(Default)]
pub struct Tkhd {
    pub flags: [u8; 3],
    pub version: TkhdVersion,
    pub track_id: u32,
    pub reserved0: u32,
    pub reserved1: [u32; 2],
    pub layer: i16,           // template=0
    pub alternate_group: i16, // template=0
    pub volume: i16,          // template={if track_is_audio 0x0100 else 0}
    pub reserved2: u16,
    pub matrix: [i32; 9], // template={ 0x00010000,0,0,0,0x00010000,0,0,0,0x40000000 };
    pub width: u32,       // fixed-point 16.16
    pub height: u32,      // fixed-point 16.16
}

pub enum TkhdVersion {
    V0(TkhdV0),
    V1(TkhdV1),
}

impl Default for TkhdVersion {
    fn default() -> Self {
        Self::V0(TkhdV0::default())
    }
}

#[derive(Default)]
pub struct TkhdV0 {
    pub creation_time: u32,
    pub modification_time: u32,
    pub duration: u32,
}

pub struct TkhdV1 {
    pub creation_time: u64,
    pub modification_time: u64,
    pub duration: u64,
}

impl ImmutableBox for Tkhd {
    fn box_type(&self) -> BoxType {
        TYPE_TKHD
    }

    fn size(&self) -> usize {
        match self.version {
            TkhdVersion::V0(_) => 84,
            TkhdVersion::V1(_) => 96,
        }
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        match &self.version {
            TkhdVersion::V0(v) => {
                w.write_all(&[0])?;
                w.write_all(&self.flags)?;
                w.write_all(&v.creation_time.to_be_bytes())?;
                w.write_all(&v.modification_time.to_be_bytes())?;
                w.write_all(&self.track_id.to_be_bytes())?;
                w.write_all(&self.reserved0.to_be_bytes())?;
                w.write_all(&v.duration.to_be_bytes())?;
            }
            TkhdVersion::V1(v) => {
                w.write_all(&[1])?;
                w.write_all(&self.flags)?;
                w.write_all(&v.creation_time.to_be_bytes())?;
                w.write_all(&v.modification_time.to_be_bytes())?;
                w.write_all(&self.track_id.to_be_bytes())?;
                w.write_all(&self.reserved0.to_be_bytes())?;
                w.write_all(&v.duration.to_be_bytes())?;
            }
        }

        for reserved in &self.reserved1 {
            w.write_all(&reserved.to_be_bytes())?;
        }
        w.write_all(&self.layer.to_be_bytes())?;
        w.write_all(&self.alternate_group.to_be_bytes())?;
        w.write_all(&self.volume.to_be_bytes())?;
        w.write_all(&self.reserved2.to_be_bytes())?;
        for matrix in &self.matrix {
            w.write_all(&matrix.to_be_bytes())?;
        }
        w.write_all(&self.width.to_be_bytes())?;
        w.write_all(&self.height.to_be_bytes())?;

        Ok(())
    }
}

impl From<Tkhd> for Box<dyn ImmutableBox> {
    fn from(value: Tkhd) -> Self {
        Box::new(value)
    }
}

/*************************** traf ****************************/

pub const TYPE_TRAF: BoxType = *b"traf";

pub struct Traf;

impl ImmutableBox for Traf {
    fn box_type(&self) -> BoxType {
        TYPE_TRAF
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Traf> for Box<dyn ImmutableBox> {
    fn from(value: Traf) -> Self {
        Box::new(value)
    }
}

/*************************** trak ****************************/

pub const TYPE_TRAK: BoxType = *b"trak";

pub struct Trak;

impl ImmutableBox for Trak {
    fn box_type(&self) -> BoxType {
        TYPE_TRAK
    }

    fn size(&self) -> usize {
        0
    }

    fn marshal(&self, _: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        Ok(())
    }
}

impl From<Trak> for Box<dyn ImmutableBox> {
    fn from(value: Trak) -> Self {
        Box::new(value)
    }
}

/*************************** trex ****************************/

pub const TYPE_TREX: BoxType = *b"trex";

#[derive(Default)]
pub struct Trex {
    pub full_box: FullBox,
    pub track_id: u32,
    pub default_sample_description_index: u32,
    pub default_sample_duration: u32,
    pub default_sample_size: u32,
    pub default_sample_flags: u32,
}

impl ImmutableBox for Trex {
    fn box_type(&self) -> BoxType {
        TYPE_TREX
    }

    fn size(&self) -> usize {
        24
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(&self.track_id.to_be_bytes())?;
        w.write_all(&self.default_sample_description_index.to_be_bytes())?;
        w.write_all(&self.default_sample_duration.to_be_bytes())?;
        w.write_all(&self.default_sample_size.to_be_bytes())?;
        w.write_all(&self.default_sample_flags.to_be_bytes())?;
        Ok(())
    }
}

impl From<Trex> for Box<dyn ImmutableBox> {
    fn from(value: Trex) -> Self {
        Box::new(value)
    }
}

/*************************** trun ****************************/

pub const TRUN_DATA_OFFSET_PRESENT: u32 = 0b0000_0000_0001;
pub const TRUN_FIRST_SAMPLE_FLAGS_PRESENT: u32 = 0b0000_0000_0100;
pub const TRUN_SAMPLE_DURATION_PRESENT: u32 = 0b0001_0000_0000;
pub const TRUN_SAMPLE_SIZE_PRESENT: u32 = 0b0010_0000_0000;
pub const TRUN_SAMPLE_FLAGS_PRESENT: u32 = 0b0100_0000_0000;
pub const TRUN_SAMPLE_COMPOSITION_TIME_OFFSET_PRESENT: u32 = 0b1000_0000_0000;

pub enum TrunEntries {
    V0(Vec<TrunEntryV0>),
    V1(Vec<TrunEntryV1>),
}

impl TrunEntries {
    fn len(&self) -> usize {
        match self {
            TrunEntries::V0(entries) => entries.len(),
            TrunEntries::V1(entries) => entries.len(),
        }
    }
}

pub struct TrunEntryV0 {
    pub sample_duration: u32,
    pub sample_size: u32,
    pub sample_flags: u32,
    pub sample_composition_time_offset: u32,
}

impl TrunEntryV0 {
    fn marshal_field(&self, w: &mut dyn std::io::Write, flags: [u8; 3]) -> Result<(), Mp4Error> {
        if check_fullbox_flag(flags, TRUN_SAMPLE_DURATION_PRESENT) {
            w.write_all(&self.sample_duration.to_be_bytes())?;
        }
        if check_fullbox_flag(flags, TRUN_SAMPLE_SIZE_PRESENT) {
            w.write_all(&self.sample_size.to_be_bytes())?;
        }
        if check_fullbox_flag(flags, TRUN_SAMPLE_FLAGS_PRESENT) {
            w.write_all(&self.sample_flags.to_be_bytes())?;
        }
        if check_fullbox_flag(flags, TRUN_SAMPLE_COMPOSITION_TIME_OFFSET_PRESENT) {
            w.write_all(&self.sample_composition_time_offset.to_be_bytes())?;
        }
        Ok(())
    }
}

pub struct TrunEntryV1 {
    pub sample_duration: u32,
    pub sample_size: u32,
    pub sample_flags: u32,
    pub sample_composition_time_offset: i32,
}

impl TrunEntryV1 {
    fn marshal_field(&self, w: &mut dyn std::io::Write, flags: [u8; 3]) -> Result<(), Mp4Error> {
        if check_fullbox_flag(flags, TRUN_SAMPLE_DURATION_PRESENT) {
            w.write_all(&self.sample_duration.to_be_bytes())?;
        }
        if check_fullbox_flag(flags, TRUN_SAMPLE_SIZE_PRESENT) {
            w.write_all(&self.sample_size.to_be_bytes())?;
        }
        if check_fullbox_flag(flags, TRUN_SAMPLE_FLAGS_PRESENT) {
            w.write_all(&self.sample_flags.to_be_bytes())?;
        }
        if check_fullbox_flag(flags, TRUN_SAMPLE_COMPOSITION_TIME_OFFSET_PRESENT) {
            w.write_all(&self.sample_composition_time_offset.to_be_bytes())?;
        }
        Ok(())
    }
}

pub const TYPE_TRUN: BoxType = *b"trun";

pub struct Trun {
    pub flags: [u8; 3],

    pub data_offset: i32,
    pub first_sample_flags: u32,
    pub entries: TrunEntries,
}

fn trun_field_size(fullbox_flags: [u8; 3]) -> usize {
    let mut total = 0;
    if check_fullbox_flag(fullbox_flags, TRUN_SAMPLE_DURATION_PRESENT) {
        total += 4;
    }
    if check_fullbox_flag(fullbox_flags, TRUN_SAMPLE_SIZE_PRESENT) {
        total += 4;
    }
    if check_fullbox_flag(fullbox_flags, TRUN_SAMPLE_FLAGS_PRESENT) {
        total += 4;
    }
    if check_fullbox_flag(fullbox_flags, TRUN_SAMPLE_COMPOSITION_TIME_OFFSET_PRESENT) {
        total += 4;
    }
    total
}

impl ImmutableBox for Trun {
    fn box_type(&self) -> BoxType {
        TYPE_TRUN
    }

    fn size(&self) -> usize {
        let mut total = 8;
        if check_fullbox_flag(self.flags, TRUN_DATA_OFFSET_PRESENT) {
            total += 4;
        }
        if check_fullbox_flag(self.flags, TRUN_FIRST_SAMPLE_FLAGS_PRESENT) {
            total += 4;
        }
        let field_size = trun_field_size(self.flags);
        total += field_size * self.entries.len();
        total
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        match &self.entries {
            TrunEntries::V0(_) => w.write_all(&[0])?,
            TrunEntries::V1(_) => w.write_all(&[1])?,
        }
        w.write_all(&self.flags)?;
        w.write_all(
            &u32::try_from(self.entries.len())
                .map_err(|e| Mp4Error::FromInt("trun".to_owned(), e))?
                .to_be_bytes(),
        )?;
        if check_fullbox_flag(self.flags, TRUN_DATA_OFFSET_PRESENT) {
            w.write_all(&self.data_offset.to_be_bytes())?;
        }
        if check_fullbox_flag(self.flags, TRUN_FIRST_SAMPLE_FLAGS_PRESENT) {
            w.write_all(&self.first_sample_flags.to_be_bytes())?;
        }
        match &self.entries {
            TrunEntries::V0(entries) => {
                for entry in entries {
                    entry.marshal_field(w, self.flags)?;
                }
            }
            TrunEntries::V1(entries) => {
                for entry in entries {
                    entry.marshal_field(w, self.flags)?;
                }
            }
        };
        Ok(())
    }
}

impl From<Trun> for Box<dyn ImmutableBox> {
    fn from(value: Trun) -> Self {
        Box::new(value)
    }
}

/*************************** vmhd ****************************/

pub const TYPE_VMHD: BoxType = *b"vmhd";

#[derive(Default)]
pub struct Vmhd {
    pub full_box: FullBox,
    pub graphics_mode: u16, // template=0
    pub opcolor: [u16; 3],  // template={0, 0, 0}
}

impl ImmutableBox for Vmhd {
    fn box_type(&self) -> BoxType {
        TYPE_VMHD
    }

    fn size(&self) -> usize {
        12
    }

    fn marshal(&self, w: &mut dyn std::io::Write) -> Result<(), Mp4Error> {
        self.full_box.marshal_field(w)?;
        w.write_all(&self.graphics_mode.to_be_bytes())?;
        for color in &self.opcolor {
            w.write_all(&color.to_be_bytes())?;
        }
        Ok(())
    }
}

impl From<Vmhd> for Box<dyn ImmutableBox> {
    fn from(value: Vmhd) -> Self {
        Box::new(value)
    }
}
