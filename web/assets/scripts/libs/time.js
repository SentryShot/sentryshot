const MS_SECOND = 1000;
const MS_MINUTE = MS_SECOND * 60;
const MS_HOUR = MS_MINUTE * 60;
const MS_DAY = MS_HOUR * 24;
const MS_MONTH = MS_MINUTE * 43830;
const MS_YEAR = MS_MONTH * 12;

const NS_MICROSECOND = 1000;
const NS_MILLISECOND = NS_MICROSECOND * 1000;

/**
 * @typedef {number} UnixNano
 * @typedef {number} UnixMs
 */

/**
 * @typedef Time
 * @property {() => string} format
 * @property {(ms: number) => void} setMilliseconds
 * @property {(sec: number) => void} setSeconds
 * @property {(min: number) => void} setMinutes
 * @property {(hour: number) => void} setHours
 * @property {(day: number) => void} setDate
 * @property {(month: number) => void} setMonth
 * @property {(year: number) => void} setYear
 * @property {() => void} nextMonth
 * @property {() => void} prevMonth
 * @property {() => void} nextYear
 * @property {() => void} prevYear
 * @property {() => number} getDay
 * @property {() => number} firstDayInMonth
 * @property {() => number} getMilliseconds
 * @property {() => number} getSeconds
 * @property {() => number} getMinutes
 * @property {() => number} getHours
 * @property {() => number} getDate
 * @property {() => number} getMonth
 * @property {() => number} getFullYear
 * @property {() => UnixNano} unixNano
 * @property {() => number} daysInMonth
 * @property {() => Time} clone
 */

/**
 * @param {string} timeZone
 * @returns {Time}
 */
function newTimeNow(timeZone) {
	return newTime(Date.now(), timeZone);
}

/**
 * @param {UnixMs} unixMs
 * @param {string} timeZone
 * @returns {Time}
 */
function newTime(unixMs, timeZone) {
	const d = new Date(unixMs);

	// Creating the formatter is the slow part that doesn't show in the profiler.
	const formatter = new Intl.DateTimeFormat("en-US", {
		timeZone: timeZone,
		hour12: false,
		year: "numeric",
		month: "2-digit",
		day: "2-digit",
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
		fractionalSecondDigits: 3,
	});

	/** @type {number} */
	let year;
	/** @type {number} */
	let month;
	/** @type {number} */
	let day;
	/** @type {number} */
	let hour;
	/** @type {number} */
	let minute;
	/** @type {number} */
	let second;
	/** @type {number} */
	let ms;

	const update = () => {
		const s = formatter.format(d);
		month = Number(s.slice(0, 2));
		day = Number(s.slice(3, 5));
		year = Number(s.slice(6, 10));
		hour = Number(s.slice(12, 14));
		if (hour == 24) {
			hour = 0;
		}
		minute = Number(s.slice(15, 17));
		second = Number(s.slice(18, 20));
		ms = Number(s.slice(21, 24));
	};
	update();

	const isLeapYear = (year) => {
		return (year % 4 == 0 && year % 100 != 0) || year % 400 == 0;
	};

	//  1 JAN: 31
	//  2 FEB: 28/29
	//  3 MAR: 31
	//  4 APR: 30
	//  5 MAY: 31
	//  6 JUN: 30
	//  7 JUL: 31
	//  8 AUG: 31
	//  9 SEP: 30
	// 10 OCT: 31
	// 11 NOV: 30
	// 12 DEC: 31

	/**
	 * @param {number} month
	 * @param {number} year
	 */
	const daysInMonth = (month, year) => {
		switch (month) {
			case 0:
			case 1:
			case 3:
			case 5:
			case 7:
			case 8:
			case 10:
			case 12: {
				return 31;
			}
			case 4:
			case 6:
			case 9:
			case 11: {
				return 30;
			}
			case 2: {
				return isLeapYear(year) ? 29 : 28;
			}
			default: {
				throw `invalid month: ${month}`;
			}
		}
	};

	return {
		format() {
			return (
				`${year}-${pad(month)}-${pad(day)}` +
				` ${pad(hour)}:${pad(minute)}:${pad(second)}.${pad3(ms)}`
			);
		},
		setMilliseconds(newMs) {
			if (ms === newMs) {
				return;
			}
			d.setUTCMilliseconds(newMs);
			update();
		},
		setSeconds(newSec) {
			if (second === newSec) {
				return;
			}
			d.setSeconds(newSec);
			update();
		},
		setMinutes(newMin) {
			if (minute === newMin) {
				return;
			}
			d.setTime(d.getTime() + (newMin - minute) * MS_MINUTE);
			update();
		},
		setHours(newHour) {
			if (hour === newHour) {
				return;
			}
			d.setTime(d.getTime() + (newHour - hour) * MS_HOUR);
			update();
		},
		setDate(newDay) {
			if (day === newDay) {
				return;
			}
			d.setTime(d.getTime() + (newDay - day) * MS_DAY);
			update();
		},
		setMonth(newMonth) {
			newMonth += 1;
			if (month === newMonth) {
				return;
			}

			if (day > 28) {
				d.setTime(d.getTime() + (28 - day) * MS_DAY);
				update();
			}

			const prevDay = day;
			const prevHour = hour;

			const diff = Math.abs(newMonth - month);
			let curMonth = month;
			for (let i = 0; i < diff; i++) {
				if (newMonth > month) {
					d.setTime(d.getTime() + daysInMonth(month, year) * MS_DAY);
					if (curMonth === 12) {
						curMonth = 1;
					} else {
						curMonth += 1;
					}
				} else {
					d.setTime(d.getTime() - daysInMonth(month - 1, year) * MS_DAY);
					if (curMonth === 1) {
						curMonth = 12;
					} else {
						curMonth -= 1;
					}
				}

				update();

				// Correct the day and hour every iteration.
				if (day !== prevDay || hour !== prevHour) {
					if (day !== prevDay) {
						d.setTime(d.getTime() + (prevDay - day) * MS_DAY);
					}
					if (hour !== prevHour) {
						d.setTime(d.getTime() + (prevHour - hour) * MS_HOUR);
					}
					update();
				}
			}
		},
		setYear(newYear) {
			if (year === newYear) {
				return;
			}

			// Leap day.
			if (month === 2 && day == 29 && !isLeapYear(newYear)) {
				d.setTime(d.getTime() - MS_DAY);
				update();
			}

			const prevDay = day;
			const prevHour = hour;

			d.setTime(d.getTime() + (newYear - year) * MS_YEAR);
			update();

			// Correct the day and hour.
			if (day !== prevDay || hour !== prevHour) {
				if (day !== prevDay) {
					d.setTime(d.getTime() + (prevDay - day) * MS_DAY);
				}
				if (hour !== prevHour) {
					d.setTime(d.getTime() + (prevHour - hour) * MS_HOUR);
				}
				update();
			}
		},
		nextMonth() {
			if (day > 28) {
				d.setTime(d.getTime() + (28 - day) * MS_DAY);
				update();
			}

			const prevDay = day;
			const prevHour = hour;

			d.setTime(d.getTime() + daysInMonth(month, year) * MS_DAY);
			update();

			// Correct the day and hour.
			if (day !== prevDay || hour !== prevHour) {
				if (day !== prevDay) {
					d.setTime(d.getTime() + (prevDay - day) * MS_DAY);
				}
				if (hour !== prevHour) {
					d.setTime(d.getTime() + (prevHour - hour) * MS_HOUR);
				}
				update();
			}
		},
		prevMonth() {
			if (day > 28) {
				d.setTime(d.getTime() + (28 - day) * MS_DAY);
				update();
			}

			const prevDay = day;
			const prevHour = hour;

			d.setTime(d.getTime() - daysInMonth(month - 1, year) * MS_DAY);
			update();

			// Correct the day and hour.
			if (day !== prevDay || hour !== prevHour) {
				if (day !== prevDay) {
					d.setTime(d.getTime() + (prevDay - day) * MS_DAY);
				}
				if (hour !== prevHour) {
					d.setTime(d.getTime() + (prevHour - hour) * MS_HOUR);
				}
				update();
			}
		},
		nextYear() {
			// Leap day.
			if (month === 2 && day == 29) {
				d.setTime(d.getTime() - MS_DAY);
				update();
			}

			const prevDay = day;
			const prevHour = hour;

			d.setTime(d.getTime() + MS_YEAR);
			update();

			// Correct the day and hour.
			if (day !== prevDay || hour !== prevHour) {
				if (day !== prevDay) {
					d.setTime(d.getTime() + (prevDay - day) * MS_DAY);
				}
				if (hour !== prevHour) {
					d.setTime(d.getTime() + (prevHour - hour) * MS_HOUR);
				}
				update();
			}
		},
		prevYear() {
			// Leap day.
			if (month === 2 && day == 29) {
				d.setTime(d.getTime() - MS_DAY);
				update();
			}

			const prevDay = day;
			const prevHour = hour;

			d.setTime(d.getTime() - MS_YEAR);
			update();

			// Correct the day and hour.
			if (day !== prevDay || hour !== prevHour) {
				if (day !== prevDay) {
					d.setTime(d.getTime() + (prevDay - day) * MS_DAY);
				}
				if (hour !== prevHour) {
					d.setTime(d.getTime() + (prevHour - hour) * MS_HOUR);
				}
				update();
			}
		},
		getDay() {
			return new Date(year, month - 1, day).getDay();
		},
		firstDayInMonth() {
			return new Date(year, month - 1, 1).getDay();
		},
		getMilliseconds() {
			return ms;
		},
		getSeconds() {
			return second;
		},
		getMinutes() {
			return minute;
		},
		getHours() {
			return hour;
		},
		getDate() {
			return day;
		},
		getMonth() {
			// Month starts at zero..
			return month - 1;
		},
		getFullYear() {
			return year;
		},
		unixNano() {
			return d.getTime() * NS_MILLISECOND;
		},
		daysInMonth() {
			return daysInMonth(month, year);
		},
		clone() {
			return newTime(d.getTime(), timeZone);
		},
	};
}

/**
 * @param {number} n
 * @return string
 */
function pad(n) {
	return n < 10 ? "0" + n : n;
}

/**
 * @param {number} n
 * @return string
 */
function pad3(n) {
	if (n < 10) {
		return "00" + n;
	} else if (n < 100) {
		return "0" + n;
	}
	return n;
}

/**
 * @param {Date} date
 * @param {string} timeZone
 */
function toUTC(date, timeZone) {
	try {
		const tmp1 = new Date(date.toLocaleString("en-US", { timeZone: "utc" }));
		const unix1 = tmp1.getTime();

		const tmp2 = new Date(date.toLocaleString("en-US", { timeZone: timeZone }));
		const unix2 = tmp2.getTime();

		let utc;
		if (unix1 > unix2) {
			const offset = unix1 - unix2;
			utc = new Date(date.getTime() + offset);
		} else {
			const offset = unix2 - unix1;
			utc = new Date(date.getTime() - offset);
		}
		utc.setMilliseconds(date.getMilliseconds());
		return utc;
	} catch (error) {
		alert(error);
	}
}

/**
 * @param {Date} date
 * @param {string} timeZone
 */
function fromUTC(date, timeZone) {
	try {
		const tmp1 = new Date(date.toLocaleString("en-US", { timeZone: "utc" }));
		const unix1 = tmp1.getTime();

		const tmp2 = new Date(date.toLocaleString("en-US", { timeZone: timeZone }));
		const unix2 = tmp2.getTime();

		let utc;
		if (unix1 > unix2) {
			const offset = unix1 - unix2;
			utc = new Date(date.getTime() - offset);
		} else {
			const offset = unix2 - unix1;
			utc = new Date(date.getTime() + offset);
		}
		utc.setMilliseconds(date.getMilliseconds());
		return utc;
	} catch (error) {
		alert(error);
	}
}

/**
 * @param {Date} date
 * @param {string} timeZone
 */
function fromUTC2(date, timeZone) {
	// "MM/DD/YY, hh:mm:ss"
	const str = date.toLocaleString("en-US", {
		timeZone: timeZone,
		hour12: false,
		year: "numeric",
		month: "2-digit",
		day: "2-digit",
		hour: "2-digit",
		minute: "2-digit",
		second: "2-digit",
	});

	const hh = str.slice(12, 14);
	return {
		YY: str.slice(6, 10),
		MM: str.slice(0, 2),
		DD: str.slice(3, 5),
		hh: hh == "24" ? "00" : hh,
		mm: str.slice(15, 17),
		ss: str.slice(18, 20),
	};
}

export { NS_MILLISECOND, newTime, newTimeNow, toUTC, fromUTC, fromUTC2 };
