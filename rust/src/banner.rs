use owo_colors::{OwoColorize, Rgb};
use std::io::{self, IsTerminal};

pub const LINES: [&str; 6] = [
    "  ██╗      ██████╗  █████╗ ██████╗ ███████╗██████╗ ",
    "  ██║     ██╔═══██╗██╔══██╗██╔══██╗██╔════╝██╔══██╗",
    "  ██║     ██║   ██║███████║██║  ██║█████╗  ██║  ██║",
    "  ██║     ██║   ██║██╔══██║██║  ██║██╔══╝  ██║  ██║",
    "  ███████╗╚██████╔╝██║  ██║██████╔╝███████╗██████╔╝",
    "  ╚══════╝ ╚═════╝ ╚═╝  ╚═╝╚═════╝ ╚══════╝╚═════╝ ",
];

pub const SUBTITLE: &str = "  price monitor · rust";
pub const TAGLINE: &str = "  > track game key prices on loaded.com";

const GRADIENT_START: (u8, u8, u8) = (0xFF, 0x2E, 0x2E);
const GRADIENT_END: (u8, u8, u8) = (0xFF, 0xA8, 0x3C);

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
        println!("{}", SUBTITLE.color(Rgb(0x94, 0xa3, 0xb8)).italic());
        println!("{}", TAGLINE.color(Rgb(0x64, 0x74, 0x8b)));
    } else {
        println!("{}", SUBTITLE);
        println!("{}", TAGLINE);
    }
    println!();
}
