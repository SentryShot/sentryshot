/** @typedef {number} UnixNano */

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

export { toUTC, fromUTC, fromUTC2 };
