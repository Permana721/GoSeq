

const swiperProjects = new Swiper('.projects__swiper', {
    loop: true,
    spaceBetween: 24,
    slidesPerView: 'auto',
    grabCursor: true,
    speed: 600,

    pagination: {
        el: '.swiper-pagination',
        clickable: true,
    },

    autoplay: {
        delay: 3000,
        disableOnInteraction: false,
    },
});

const tabs = document.querySelectorAll('[data-target]'),
        tabContens = document.querySelectorAll('[data-content]')

tabs.forEach((tab) => {
    tab.addEventListener('click', () => {
        const targetSelector = tab.dataset.target,
                targetContent = document.querySelector(targetSelector)

                tabContens.forEach((content) => content.classList.remove('work-active'))
                tabs.forEach((t) => t.classList.remove('work-active'))

                tab.classList.add('work-active')
                targetContent.classList.add('work-active')
    })
})

const servicesButtons = document.querySelectorAll('.services__button')

servicesButtons.forEach(button => {
	const heightInfo = document.querySelector('.services__info')
	heightInfo.style.height = heightInfo.scrollHeight + 'px'

	button.addEventListener('click', () => {
		const servicesCards = document.querySelectorAll('.services__card'),
            currentCard = button.parentNode,
            currentInfo = currentCard.querySelector('.services__info'),
            isCardOpen = currentCard.classList.contains('services-open')

		servicesCards.forEach(card => {
			card.classList.replace('services-open', 'services-close')

            const info = card.querySelector('.services__info')
            info.style.height = '0'
		})

        if(!isCardOpen){
			currentCard.classList.replace('services-close', 'services-open')
			currentInfo.style.height = currentInfo.scrollHeight + 'px'
		}
	})
})

document.addEventListener('DOMContentLoaded', () => {
    const copyBtn = document.getElementById('contact-btn');
    const emailSpan = document.getElementById('contact-email');

    copyBtn.addEventListener('click', () => {
        const copyEmail = emailSpan.innerText; 

        navigator.clipboard.writeText(copyEmail).then(() => {
            copyBtn.innerHTML = 'Email copied <i class="ri-check-line"></i>';

            setTimeout(() => {
                copyBtn.innerHTML = 'Copy email <i class="ri-file-copy-line"></i>';
            }, 2000);
        }).catch(err => {
            console.error('Gagal menyalin: ', err);
        });
    });
});

const textYear = document.getElementById('footer-year'),
    currentYear = new Date().getFullYear()

textYear.textContent = currentYear

const sections = document.querySelectorAll('section[id]')

const scrollActive = () => {
	const scrollY = window.scrollY

	sections.forEach(section => {
		const id = section.id,
                top = section.offsetTop - 58, 
                height = section.offsetHeight, 
                link = document.querySelector('.nav__menu a[href*=' + id + ']')

		if(!link) return

		link.classList.toggle('active-link', scrollY > top && scrollY <= top + height)
	})
}
window.addEventListener('scroll', scrollActive)

const sr = ScrollReveal({
	origin: 'top',
	distance: '60px',
	duration: 2000,
	delay: 300,
})

sr.reveal(`.home__image, .projects__container, .work__container,
		.testimonials__container, .contact__container`)
sr.reveal(`.home__data`, {delay: 900, origin: 'bottom'})
sr.reveal(`.home__social`, {delay: 1500})
sr.reveal(`.about__data`, {origin: 'left'})
sr.reveal(`.about__image`, {origin: 'right'})
sr.reveal(`.services__card`, {interval: 100})