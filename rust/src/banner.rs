use owo_colors::{OwoColorize, Rgb};
use std::io::{self, IsTerminal};

pub const LINES: [&str; 12] = [
    "  ██╗      ██████╗  █████╗ ██████╗ ███████╗██████╗ ",
    "  ██║     ██╔═══██╗██╔══██╗██╔══██╗██╔════╝██╔══██╗",
    "  ██║     ██║   ██║███████║██║  ██║█████╗  ██║  ██║",
    "  ██║     ██║   ██║██╔══██║██║  ██║██╔══╝  ██║  ██║",
    "  ███████╗╚██████╔╝██║  ██║██████╔╝███████╗██████╔╝",
    "  ╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═════╝ ╚══════╝╚═════╝ ",
    "  ██████╗ ██████╗ ██╗ ██████╗███████╗    ███╗   ███╗ ██████╗ ███╗   ██╗██╗████████╗ ██████╗ ██████╗ ",
    "  ██╔══██╗██╔══██╗██║██╔════╝██╔════╝    ████╗ ████║██╔═══██╗████╗  ██║██║╚══██╔══╝██╔═══██╗██╔══██╗",
    "  ██████╔╝██████╔╝██║██║     █████╗      ██╔████╔██║██║   ██║██╔██╗ ██║██║   ██║   ██║   ██║██████╔╝",
    "  ██╔═══╝ ██╔══██╗██║██║     ██╔══╝      ██║╚██╔╝██║██║   ██║██║╚██╗██║██║   ██║   ██║   ██║██╔══██╗",
    "  ██║     ██║  ██║██║╚██████╗███████╗    ██║ ╚═╝ ██║╚██████╔╝██║ ╚████║██║   ██║   ╚██████╔╝██║  ██║",
    "  ╚═╝     ╚═╝  ╚═╝╚═╝ ╚═════╝╚══════╝    ╚═╝     ╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝   ╚═╝    ╚═════╝╚═╝  ╚═╝",
];

pub const TAGLINE: &str = "  > track game key prices on loaded.com";

// cyan (#22d3ee) -> magenta (#d946ef), matching khinsider-scraper
const GRADIENT_START: (u8, u8, u8) = (0x22, 0xD3, 0xEE);
const GRADIENT_END: (u8, u8, u8) = (0xD9, 0x46, 0xEF);

fn color_enabled() -> bool {
    if std::env::var_os("NO_COLOR").is_some() {
        return false;
    }
    if std::env::var_os("FORCE_COLOR").is_some()
        || std::env::var_os("CLICOLOR_FORCE").is_some()
    {
        return true;
    }
    io::stdout().is_terminal()
}

fn lerp(a: u8, b: u8, t: f32) -> u8 {
    let v = (a as f32 + (b as f32 - a as f32) * t).round();
    v.clamp(0.0, 255.0) as u8
}

fn gradient_color(idx: usize, total: usize) -> Rgb {
    let t = if total <= 1 { 0.0 } else { idx as f32 / (total - 1) as f32 };
    Rgb(
        lerp(GRADIENT_START.0, GRADIENT_END.0, t),
        lerp(GRADIENT_START.1, GRADIENT_END.1, t),
        lerp(GRADIENT_START.2, GRADIENT_END.2, t),
    )
}

pub fn print_banner(force: bool) {
    let enabled = color_enabled() || force;
    if !enabled && !io::stdout().is_terminal() && !force {
        return;
    }
    println!();
    let total = LINES.len();
    for (i, line) in LINES.iter().enumerate() {
        if enabled {
            let c = gradient_color(i, total);
            println!("{}", line.color(c).bold());
        } else {
            println!("{}", line);
        }
    }
    if enabled {
        println!("{}", TAGLINE.color(Rgb(0x64, 0x74, 0x8b)));
    } else {
        println!("{}", TAGLINE);
    }
    println!();
}
